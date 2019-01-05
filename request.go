/*
 * Datadog API for Go
 *
 * Please see the included LICENSE file for licensing information.
 *
 * Copyright 2013 by authors and contributors.
 */

package datadog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff"
)

// StatusResponse contains common fields that might be present in any API response.
type StatusResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

type ResponseMetadata struct {
	RateLimit RateLimit
}

// uriForAPI is to be called with something like "/v1/events" and it will give
// the proper request URI to be posted to.
func (client *Client) uriForAPI(api string) (string, error) {
	apiBase, err := url.Parse(client.baseUrl + "/api" + api)
	if err != nil {
		return "", err
	}
	q := apiBase.Query()
	q.Add("api_key", client.apiKey)
	q.Add("application_key", client.appKey)
	apiBase.RawQuery = q.Encode()
	return apiBase.String(), nil
}

// redactKeysFromError removes api and application keys from error strings
func redactKeysFromError(err error, keys ...string) error {
	if err == nil {
		return nil
	}
	errMessage := err.Error()

	for _, key := range keys {
		if len(key) > 0 {
			errMessage = strings.Replace(errMessage, key, "redacted", -1)
		}
	}

	// Return original error if no replacements were made to keep the original,
	// probably more useful error type information.
	if errMessage == err.Error() {
		return err
	}
	return errors.New(errMessage)
}

// doJsonRequest is the simplest type of request: a method on a URI that
// returns some JSON result which we unmarshal into the passed interface. It
// wraps doJsonRequestUnredacted to redact api and application keys from
// errors.
func (client *Client) doJsonRequest(method, api string, reqBody, out interface{}) error {
	_, err := client.doRequestWithContext(nil, method, api, reqBody, out)
	return err
}

func (client *Client) doRequestWithContext(
	ctx context.Context,
	method string,
	api string,
	reqBody interface{},
	out interface{},
) (ResponseMetadata, error) {
	md := ResponseMetadata{}
	url, err := client.uriForAPI(api)
	if err != nil {
		return md, err
	}
	req, err := newJSONRequest(method, url, reqBody)
	if err != nil {
		return md, redactKeysFromError(err, client.apiKey, client.appKey)
	}
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	resp, err := doerForMethod(client, method)(req)
	if err != nil {
		return md, redactKeysFromError(err, client.apiKey, client.appKey)
	}
	err = handleResponse(resp, out)
	resp.Body.Close()
	if err != nil {
		return md, redactKeysFromError(err, client.apiKey, client.appKey)
	}
	md.RateLimit = newRateLimitFromHeaders(resp.Header)
	return md, nil
}

func newJSONRequest(method, url string, reqBody interface{}) (*http.Request, error) {
	body, err := encodeRequestBody(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Add("Content-Type", "application/json")
	}
	return req, nil
}

// encodeRequestBody into a buffer that can be reused if the request is retried.
func encodeRequestBody(body interface{}) (io.Reader, error) {
	if body == nil {
		return nil, nil
	}
	raw, err := json.Marshal(body)
	return bytes.NewReader(raw), err
}

type doer func(req *http.Request) (*http.Response, error)

func doerForMethod(client *Client, method string) doer {
	if method == "POST" || method == "PUT" {
		return client.HttpClient.Do
	}
	return client.doRequestWithRetries
}

// doRequestWithRetries performs an HTTP request repeatedly for maxTime or until
// an error or non-retryable HTTP response code is received.
func (client *Client) doRequestWithRetries(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	operation := func() error {
		var err error
		resp, err = client.HttpClient.Do(req)
		switch {
		case err != nil:
			return err
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return nil
		case resp.StatusCode >= 400 && resp.StatusCode < 500:
			return nil
		default:
			return fmt.Errorf("Received HTTP status code %d", resp.StatusCode)
		}
	}
	backOff := backoff.WithContext(client.getBackOff(), req.Context())
	return resp, backoff.RetryNotify(operation, backOff, client.RetryNotify)
}

func (client *Client) getBackOff() backoff.BackOff {
	if client.BackOff != nil {
		return client.BackOff
	}
	expBackOff := backoff.NewExponentialBackOff()
	switch {
	case client.RetryTimeout != -1:
		expBackOff.MaxElapsedTime = client.RetryTimeout
	default:
		expBackOff.MaxElapsedTime = 60 * time.Second
	}
	return expBackOff
}

// handleResponse reports errors if it finds any, otherwise unmarshals the
// response body into out.
func handleResponse(resp *http.Response, out interface{}) error {
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("API error %s: %s", resp.Status, body)
	}
	if len(body) == 0 {
		body = []byte{'{', '}'}
	}

	// Try to parse common response fields to check whether there's an error reported in a response.
	var common StatusResponse
	err = json.Unmarshal(body, &common)
	if err != nil {
		// UnmarshalTypeErrors are ignored, because in some cases API response is an array that cannot be
		// unmarshalled into a struct.
		// TODO: if the API is returning different types for common, maybe it's
		// not so common after all. Why are some of these errors ignored?
		_, ok := err.(*json.UnmarshalTypeError)
		if !ok {
			return err
		}
	}
	if common.Status == "error" {
		return fmt.Errorf("API returned error: %s", common.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, &out)
}
