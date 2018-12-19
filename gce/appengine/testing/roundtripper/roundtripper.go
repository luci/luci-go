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

// Package roundtripper contains an http.RoundTripper implementation suitable for testing.
package roundtripper

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"

	"go.chromium.org/luci/common/errors"
)

// Ensure *JSONRoundTripper implements http.RoundTripper.
var _ http.RoundTripper = (*JSONRoundTripper)(nil)

// JSONRoundTripper implements http.RoundTripper to handle *http.Requests with a JSON body.
type JSONRoundTripper struct {
	// Handler is called by RoundTrip with the unmarshalled JSON from an *http.Request.
	// Returns an HTTP status code and an interface{} to marshal as JSON in an *http.Response.
	Handler func(interface{}) (int, interface{})
	// Type is the reflect.Type to unmarshal *http.Request.Body into.
	// Defaults to map[string]string{}.
	Type reflect.Type
}

// RoundTrip handles an *http.Request with a JSON body, returning an *http.Response with a JSON body.
// Panics on error. Implements http.RoundTripper.
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
			panic(errors.Annotate(err, "failed to unmarshal *http.Request.Body").Err())
		}
	}

	// Marshal the returned value from t.Handler into *http.Response.Body.
	if t.Handler == nil {
		panic("handler is required")
	}
	code, rsp := t.Handler(val)
	b, err := json.Marshal(rsp)
	if err != nil {
		panic(errors.Annotate(err, "failed to marshal *http.Response.Body").Err())
	}
	return &http.Response{
		Body:       ioutil.NopCloser(bytes.NewReader(b)),
		StatusCode: code,
	}, nil
}
