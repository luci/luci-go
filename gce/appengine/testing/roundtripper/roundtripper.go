// Copyright 2018 The LUCI Authors.
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

// Package roundtripper contains http.RoundTripper implementations suitable for
// testing.
package roundtripper

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Ensure *StringRoundTripper implements http.RoundTripper.
var _ http.RoundTripper = &StringRoundTripper{}

// StringRoundTripper implements http.RoundTripper to handle *http.Requests.
type StringRoundTripper struct {
	// Handler is called by RoundTrip.
	// Returns an HTTP status code and a string to return in an *http.Response.
	Handler func(*http.Request) (int, string)
}

// RoundTrip handles an *http.Request, returning an *http.Response with a string
// body. Panics on error. Implements http.RoundTripper.
func (t *StringRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Handler == nil {
		panic("handler is required")
	}
	code, rsp := t.Handler(req)
	return &http.Response{
		Body:       io.NopCloser(strings.NewReader(rsp)),
		StatusCode: code,
	}, nil
}

// Ensure *JSONRoundTripper implements http.RoundTripper.
var _ http.RoundTripper = &JSONRoundTripper{}

// JSONRoundTripper implements http.RoundTripper to handle *http.Requests with a
// JSON body.
type JSONRoundTripper struct {
	// Handler is called by RoundTrip with the unmarshalled JSON from an *http.Request.
	// Returns an HTTP status code and an any to marshal as JSON in an *http.Response.
	Handler func(any) (int, any)
	// Type is the reflect.Type to unmarshal *http.Request.Body into.
	// Defaults to map[string]string{}.
	Type reflect.Type
}

// RoundTrip handles an *http.Request with a JSON body, returning an
// *http.Response with a JSON body. Panics on error. Implements
// http.RoundTripper.
func (t *JSONRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Unmarshal *http.Request.Body.
	if t.Type == nil {
		t.Type = reflect.TypeOf(map[string]string{})
	}
	val := reflect.New(t.Type).Interface()
	if req.Body != nil {
		err := json.NewDecoder(req.Body).Decode(&val)
		req.Body.Close()
		if err != nil {
			panic(errors.Fmt("failed to unmarshal *http.Request.Body: %w", err))
		}
	}

	// Marshal the returned value from t.Handler into *http.Response.Body.
	if t.Handler == nil {
		panic("handler is required")
	}
	code, rsp := t.Handler(val)
	b, err := json.Marshal(rsp)
	if err != nil {
		panic(errors.Fmt("failed to marshal *http.Response.Body: %w", err))
	}
	return &http.Response{
		Body:       io.NopCloser(bytes.NewReader(b)),
		StatusCode: code,
	}, nil
}
