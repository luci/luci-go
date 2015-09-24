// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lhttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
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

// NewRequest returns a retriable request.
//
// To enable automatic retry support, the Request.Body, if present, must
// implement io.Seeker.
//
// handler should return retry.Error in case of retriable error, for example if
// a TCP connection is teared off while receiving the content.
func NewRequest(c *http.Client, req *http.Request, handler Handler) (Retriable, error) {
	// Handle req.Body if specified. It has to implement io.Seeker.
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported protocol scheme \"%s\"", req.URL.Scheme)
	}
	newReq := *req
	out := &retriable{
		handler:   handler,
		c:         c,
		req:       &newReq,
		closeBody: req.Body,
	}
	if req.Body != nil {
		ok := false
		if out.seekBody, ok = req.Body.(io.Seeker); !ok {
			return nil, errors.New("req.Body must implement io.Seeker")
		}
		// Make sure the body is not closed when calling http.Client.Do().
		out.req.Body = ioutil.NopCloser(req.Body)
	}
	return out, nil
}

// NewRequestJSON returns a retriable request calling a JSON endpoint.
func NewRequestJSON(c *http.Client, url, method string, in, out interface{}) (Retriable, error) {
	var body io.Reader
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = newReader(encoded)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if in != nil {
		req.Header.Set("Content-Type", jsonContentType)
	}
	return NewRequest(c, req, func(resp *http.Response) error {
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
	})
}

// GetJSON is a shorthand. It returns the HTTP status code and error if any.
func GetJSON(config *retry.Config, c *http.Client, url string, out interface{}) (int, error) {
	req, err := NewRequestJSON(c, url, "GET", nil, out)
	if err != nil {
		return 0, err
	}
	err = config.Do(req)
	return req.Status(), err
}

// PostJSON is a shorthand. It returns the HTTP status code and error if any.
func PostJSON(config *retry.Config, c *http.Client, url string, in, out interface{}) (int, error) {
	req, err := NewRequestJSON(c, url, "POST", in, out)
	if err != nil {
		return 0, err
	}
	err = config.Do(req)
	return req.Status(), err
}

// Private details.

const jsonContentType = "application/json; charset=utf-8"

// newReader returns a io.ReadCloser compatible read-only buffer that also
// exposes io.Seeker. This should be used instead of bytes.NewReader(), which
// doesn't implement Close().
func newReader(p []byte) io.ReadCloser {
	return &reader{bytes.NewReader(p)}
}

type reader struct {
	*bytes.Reader
}

func (r *reader) Close() error {
	return nil
}

type retriable struct {
	handler   Handler
	c         *http.Client
	req       *http.Request
	closeBody io.Closer
	seekBody  io.Seeker
	status    int
}

func (r *retriable) Close() error {
	if r.closeBody != nil {
		return r.closeBody.Close()
	}
	return nil
}

// Warning: it returns an error on HTTP >=400. This is different than
// http.Client.Do() but hell it makes coding simpler.
func (r *retriable) Do() error {
	//log.Printf("Do %s", r.req.URL)
	if r.seekBody != nil {
		if _, err := r.seekBody.Seek(0, os.SEEK_SET); err != nil {
			// Can't be retried.
			return err
		}
	}
	resp, err := r.c.Do(r.req)
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
	if resp.StatusCode == 404 && strings.HasPrefix(r.req.URL.Path, "/_ah/api/") && resp.Header.Get("Content-Type") == "text/html" {
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
