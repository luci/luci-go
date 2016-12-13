// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package lhttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
)

const (
	jsonContentType        = "application/json"
	jsonContentTypeForPOST = "application/json; charset=utf-8"
)

// Handler is called once or multiple times for each HTTP request that is tried.
type Handler func(*http.Response) error

// RequestGen is a generator function to create a new request. It may be called
// multiple times if an operation needs to be retried. The HTTP server is
// responsible for closing the Request body, as per http.Request Body method
// documentation.
type RequestGen func() (*http.Request, error)

// NewRequest returns a retriable request.
//
// The handler func is responsible for closing the response Body before
// returning. It should return retry.Error in case of retriable error, for
// example if a TCP connection is terminated while receiving the content.
func NewRequest(ctx context.Context, c *http.Client, rFn retry.Factory, rgen RequestGen, handler Handler) func() (int, error) {
	if rFn == nil {
		rFn = retry.Default
	}

	return func() (int, error) {
		status, attempts := 0, 0
		err := retry.Retry(ctx, rFn, func() error {
			attempts++
			req, err := rgen()
			if err != nil {
				return err
			}

			resp, err := c.Do(req)
			if err != nil {
				// Retry every error. This is sad when you specify an invalid hostname but
				// it's better than failing when DNS resolution is flaky.
				return errors.WrapTransient(err)
			}
			status = resp.StatusCode

			switch {
			case status == 408, status == 429, status >= 500:
				// The HTTP status code means the request should be retried.
				err = errors.WrapTransient(
					fmt.Errorf("http request failed: %s (HTTP %d)", http.StatusText(status), status))
			case status == 404 && strings.HasPrefix(req.URL.Path, "/_ah/api/"):
				// Endpoints occasionally return 404 on valid requests!
				logging.Infof(ctx, "lhttp.Do() got a Cloud Endpoints 404: %#v", resp.Header)
				err = errors.WrapTransient(
					fmt.Errorf("http request failed (endpoints): %s (HTTP %d)", http.StatusText(status), status))
			case status >= 400:
				// Any other failure code is a hard failure.
				err = fmt.Errorf("http request failed: %s (HTTP %d)", http.StatusText(status), status)
			default:
				// The handler may still return a retry.Error to indicate that the request
				// should be retried even on successful status code.
				return handler(resp)
			}

			// Drain and close the resp.Body.
			io.Copy(ioutil.Discard, resp.Body)
			resp.Body.Close()
			return err
		}, nil)
		if err != nil {
			err = fmt.Errorf("%v (attempts: %d)", err, attempts)
		}
		return status, err
	}
}

// NewRequestJSON returns a retriable request calling a JSON endpoint.
func NewRequestJSON(ctx context.Context, c *http.Client, rFn retry.Factory, url, method string, headers map[string]string, in, out interface{}) (func() (int, error), error) {
	var encoded []byte
	if in != nil {
		var err error
		if encoded, err = json.Marshal(in); err != nil {
			return nil, err
		}
	}

	return NewRequest(ctx, c, rFn, func() (*http.Request, error) {
		var body io.Reader
		if encoded != nil {
			body = bytes.NewReader(encoded)
		}

		req, err := http.NewRequest(method, url, body)
		if err != nil {
			return nil, err
		}
		if encoded != nil {
			req.Header.Set("Content-Type", jsonContentTypeForPOST)
		}
		if headers != nil {
			for k, v := range headers {
				req.Header.Add(k, v)
			}
		}
		return req, nil
	}, func(resp *http.Response) error {
		defer resp.Body.Close()
		if ct := strings.ToLower(resp.Header.Get("Content-Type")); !strings.HasPrefix(ct, jsonContentType) {
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
			return errors.WrapTransient(fmt.Errorf("bad response %s: %s", url, err))
		}
		return nil
	}), nil
}

// GetJSON is a shorthand. It returns the HTTP status code and error if any.
func GetJSON(ctx context.Context, rFn retry.Factory, c *http.Client, url string, out interface{}) (int, error) {
	req, err := NewRequestJSON(ctx, c, rFn, url, "GET", nil, nil, out)
	if err != nil {
		return 0, err
	}
	return req()
}

// PostJSON is a shorthand. It returns the HTTP status code and error if any.
func PostJSON(ctx context.Context, rFn retry.Factory, c *http.Client, url string, headers map[string]string, in, out interface{}) (int, error) {
	req, err := NewRequestJSON(ctx, c, rFn, url, "POST", headers, in, out)
	if err != nil {
		return 0, err
	}
	return req()
}
