// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lhttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/luci/luci-go/client/internal/retry"
)

// Handler is called once or multiple times for each HTTP request that is tried.
type Handler func(*http.Response) error

// Retriable is a retry.Retriable that exposes the resulting HTTP status
// code.
type Retriable interface {
	retry.Retriable
	// Returns the HTTP status code of the last request, if set.
	Status() int
}

// RequestGen is a generator function to create a new request. It may be called
// multiple times if an operation needs to be retried. The HTTP server is
// responsible for closing the Request body, as per http.Request Body method
// documentation.
type RequestGen func() (*http.Request, error)

// NewRequest returns a retriable request.
//
// To enable automatic retry support, the Request.Body, if present, must
// implement io.Seeker.
//
// handler should return retry.Error in case of retriable error, for example if
// a TCP connection is teared off while receiving the content.
func NewRequest(c *http.Client, rgen RequestGen, handler Handler) Retriable {
	return &retriable{
		handler: handler,
		c:       c,
		rgen:    rgen,
	}
}

// NewRequestJSON returns a retriable request calling a JSON endpoint.
func NewRequestJSON(c *http.Client, url, method string, headers map[string]string, in, out interface{}) (Retriable, error) {
	var encoded []byte
	if in != nil {
		var err error
		if encoded, err = json.Marshal(in); err != nil {
			return nil, err
		}
	}

	return NewRequest(c, func() (*http.Request, error) {
		var body io.Reader
		if encoded != nil {
			body = bytes.NewReader(encoded)
		}

		req, err := http.NewRequest(method, url, body)
		if err != nil {
			return nil, err
		}
		if encoded != nil {
			req.Header.Set("Content-Type", jsonContentType)
		}
		if headers != nil {
			for k, v := range headers {
				req.Header.Add(k, v)
			}
		}
		return req, nil
	}, func(resp *http.Response) error {
		defer resp.Body.Close()
		if ct := strings.ToLower(resp.Header.Get("Content-Type")); ct != jsonContentType {
			// Non-retriable.
			return fmt.Errorf("unexpected Content-Type, expected \"%s\", got \"%s\"", jsonContentType, ct)
		}
		if out == nil {
			// The client doesn't care about the response. Still ensure the response
			// is valid json.
			out = &map[string]interface{}{}
		}
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			// Retriable.
			return retry.Error{fmt.Errorf("bad response %s: %s", url, err)}
		}
		return nil
	}), nil
}

// GetJSON is a shorthand. It returns the HTTP status code and error if any.
func GetJSON(config *retry.Config, c *http.Client, url string, out interface{}) (int, error) {
	req, err := NewRequestJSON(c, url, "GET", nil, nil, out)
	if err != nil {
		return 0, err
	}
	err = config.Do(req)
	return req.Status(), err
}

// PostJSON is a shorthand. It returns the HTTP status code and error if any.
func PostJSON(config *retry.Config, c *http.Client, url string, headers map[string]string, in, out interface{}) (int, error) {
	req, err := NewRequestJSON(c, url, "POST", headers, in, out)
	if err != nil {
		return 0, err
	}
	err = config.Do(req)
	return req.Status(), err
}

// Private details.

const jsonContentType = "application/json; charset=utf-8"

type retriable struct {
	handler Handler
	c       *http.Client
	rgen    RequestGen
	try     int
	status  int
}

func (r *retriable) Close() error { return nil }

// Warning: it returns an error on HTTP >=400. This is different than
// http.Client.Do() but hell it makes coding simpler.
func (r *retriable) Do() error {
	req, err := r.rgen()
	if err != nil {
		return err
	}

	r.try++
	resp, err := r.c.Do(req)
	if resp != nil {
		r.status = resp.StatusCode
	} else {
		r.status = 0
	}
	if err != nil {
		// Retry every error. This is sad when you specify an invalid hostname but
		// it's better than failing when DNS resolution is flaky.
		return retry.Error{err}
	}
	// If the HTTP status code means the request should be retried.
	if resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500 {
		return retry.Error{fmt.Errorf("http request failed: %s (HTTP %d)", http.StatusText(resp.StatusCode), resp.StatusCode)}
	}
	// Endpoints occasionally return 404 on valid requests (!)
	if resp.StatusCode == 404 && strings.HasPrefix(req.URL.Path, "/_ah/api/") {
		log.Printf("lhttp.Do() got a Cloud Endpoints 404: %#v", resp.Header)
		return retry.Error{fmt.Errorf("http request failed (endpoints): %s (HTTP %d)", http.StatusText(resp.StatusCode), resp.StatusCode)}
	}
	// Any other failure code is a hard failure.
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http request failed: %s (HTTP %d)", http.StatusText(resp.StatusCode), resp.StatusCode)
	}
	// The handler may still return a retry.Error to indicate that the request
	// should be retried even on successful status code.
	return r.handler(resp)
}

func (r *retriable) Status() int {
	return r.status
}
