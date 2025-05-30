// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lhttp

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

// Handler is called once or multiple times for each HTTP request that is tried.
type Handler func(*http.Response) error

// ErrorHandler is called once or multiple times for each HTTP request that is
// tried.  It is called when any non-200 response code is received, or if some
// other network error occurs.
// resp may be nil if a network error occurred before the response was received.
// The ErrorHandler must close the provided resp, if any.
// Return the same error again to continue retry behaviour, or nil to pretend
// this error was a success.
type ErrorHandler func(resp *http.Response, err error) error

// RequestGen is a generator function to create a new request. It may be called
// multiple times if an operation needs to be retried. The HTTP server is
// responsible for closing the Request body, as per http.Request Body method
// documentation.
type RequestGen func() (*http.Request, error)

var httpTag = errtag.Make("this is an HTTP error", 0)

func IsHTTPError(err error) (status int, ok bool) {
	return httpTag.Value(err)
}

// NewRequest returns a retriable request.
//
// The handler func is responsible for closing the response Body before
// returning. It should return retry.Error in case of retriable error, for
// example if a TCP connection is terminated while receiving the content.
//
// If rFn is nil, NewRequest will use a default exponential backoff strategy
// only for transient errors.
//
// If errorHandler is nil, the default error handler will drain and close the
// response body.
func NewRequest(ctx context.Context, c *http.Client, rFn retry.Factory, rgen RequestGen,
	handler Handler, errorHandler ErrorHandler) func() (int, error) {
	if rFn == nil {
		rFn = transient.Only(retry.Default)
	}
	if errorHandler == nil {
		errorHandler = func(resp *http.Response, err error) error {
			if resp != nil {
				// Drain and close the resp.Body.
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			return err
		}
	}

	return func() (int, error) {
		status, attempts := 0, 0
		err := retry.Retry(ctx, rFn, func() error {
			attempts++
			req, err := rgen()
			if err != nil {
				return errors.Fmt("failed to call rgen: %w", err)
			}

			resp, err := c.Do(req)
			if err != nil {
				logging.Debugf(ctx, "failed to call c.Do: %v", err)
				err = errors.Fmt("failed to call c.Do: %w", err)
				// Retry every error. This is sad when you specify an invalid hostname but
				// it's better than failing when DNS resolution is flaky.
				return errorHandler(nil, transient.Tag.Apply(err))
			}
			status = resp.StatusCode

			switch {
			case status == 408, status == 429, status >= 500:
				// The HTTP status code means the request should be retried.
				err =
					transient.Tag.Apply(errors.Fmt("http request failed: %s (HTTP %d)", http.StatusText(status), status))

			case status >= 400:
				// Any other failure code is a hard failure.
				err = fmt.Errorf("http request failed: %s (HTTP %d)", http.StatusText(status), status)
			default:
				// The handler may still return a retry.Error to indicate that the request
				// should be retried even on successful status code.
				err = handler(resp)
				if err != nil {
					return errors.Fmt("failed to handle response: %w", err)
				}
				return err
			}

			err = httpTag.ApplyValue(err, status)
			return errorHandler(resp, err)
		}, nil)
		if err != nil {
			err = errors.Fmt("gave up after %d attempts: %w", attempts, err)
		}
		return status, err
	}
}
