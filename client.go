/*
 * Datadog API for Go
 *
 * Please see the included LICENSE file for licensing information.
 *
 * Copyright 2013 by authors and contributors.
 */

package datadog

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/cenkalti/backoff"
)

// Client is the object that handles talking to the Datadog API. This maintains
// state information for a particular application connection.
type Client struct {
	apiKey, appKey, baseUrl string

	// HttpClient used to perform API requests. A default will be used if
	// none is provided. Note the default has no timeout.
	HttpClient *http.Client
	// RetryTimeout used when no BackOff is set.
	RetryTimeout time.Duration
	// BackOff used to retry failed requests.
	BackOff backoff.BackOff
	// RetryNotify is called one each retry.
	RetryNotify backoff.Notify
}

// valid is the struct to unmarshal validation endpoint responses into.
type valid struct {
	Errors  []string `json:"errors"`
	IsValid bool     `json:"valid"`
}

// NewClient returns a new datadog.Client which can be used to access the API
// methods. The expected argument is the API key.
func NewClient(apiKey, appKey string) *Client {
	baseUrl := os.Getenv("DATADOG_HOST")
	if baseUrl == "" {
		baseUrl = "https://app.datadoghq.com"
	}

	return &Client{
		apiKey:       apiKey,
		appKey:       appKey,
		baseUrl:      baseUrl,
		HttpClient:   http.DefaultClient,
		RetryTimeout: -1,
	}
}

// SetKeys changes the value of apiKey and appKey.
func (c *Client) SetKeys(apiKey, appKey string) {
	c.apiKey = apiKey
	c.appKey = appKey
}

// SetBaseUrl changes the value of baseUrl.
func (c *Client) SetBaseUrl(baseUrl string) {
	c.baseUrl = baseUrl
}

// GetBaseUrl returns the baseUrl.
func (c *Client) GetBaseUrl() string {
	return c.baseUrl
}

// Validate checks if the API and application keys are valid.
func (client *Client) Validate() (bool, error) {
	var out valid
	var resp *http.Response

	uri, err := client.uriForAPI("/v1/validate")
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return false, err
	}

	resp, err = client.doRequestWithRetries(req)
	if err != nil {
		return false, err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if err = json.Unmarshal(body, &out); err != nil {
		return false, err
	}

	return out.IsValid, nil
}
