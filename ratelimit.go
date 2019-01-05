package datadog

import (
	"log"
	"net/http"
	"strconv"
	"time"
)

// RateLimit contains details from an API response about how many requests
// remain before a request will be rate limited, or when the next request
// can be made.
type RateLimit struct {
	// Limit is the number of requests allowed in a time period.
	Limit int
	// Period is the length of time in seconds before the limit is reset.
	Period time.Duration
	// Remaining number of allowed requests left in current time period.
	Remaining int
	// Reset is the time in seconds until the limit is reset.
	Reset time.Duration
}

func newRateLimitFromHeaders(header http.Header) RateLimit {
	return RateLimit{
		Limit:     intFromHeader(header, "X-RateLimit-Limit"),
		Period:    durationFromHeader(header, "X-RateLimit-Period"),
		Remaining: intFromHeader(header, "X-RateLimit-Remaining"),
		Reset:     durationFromHeader(header, "X-RateLimit-Reset"),
	}
}

func intFromHeader(header http.Header, key string) int {
	value, err := strconv.ParseInt(header.Get(key), 10, 64)
	if err != nil {
		log.Printf("failed to parse rate limit header %v: %v", key, err)
		return 0
	}
	return int(value)
}

// TODO: are these floats or ints?
func durationFromHeader(header http.Header, key string) time.Duration {
	seconds, err := strconv.ParseFloat(header.Get(key), 64)
	if err != nil {
		log.Printf("failed to parse rate limit header %v: %v", key, err)
		return 0
	}
	return time.Duration(seconds * 1e9)
}
