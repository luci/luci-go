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

package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/net/context/ctxhttp"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
)

// ClientFactory knows how to produce http.Client that attach proper OAuth
// headers.
//
// If 'scopes' is empty, the factory should return a client that makes anonymous
// requests.
type ClientFactory func(ctx context.Context, scopes []string) (*http.Client, error)

var clientFactory ClientFactory

// RegisterClientFactory allows external module to provide implementation of
// the ClientFactory.
//
// This is needed to resolve module dependency cycle between server/auth and
// server/auth/internal.
//
// See init() in server/auth/client.go.
//
// If client factory is not set, Do(...) uses http.DefaultClient. This happens
// in unit tests for various auth/* subpackages.
func RegisterClientFactory(f ClientFactory) {
	if clientFactory != nil {
		panic("ClientFactory is already registered")
	}
	clientFactory = f
}

// Request represents one JSON REST API request.
type Request struct {
	Method  string            // HTTP method to use
	URL     string            // URL to access
	Scopes  []string          // OAuth2 scopes to authenticate with or anonymous call if empty
	Headers map[string]string // optional map with request headers
	Body    any               // object to convert to JSON and send as body or []byte with the body
	Out     any               // where to deserialize the response to
}

// Do performs an HTTP request with retries on transient errors.
//
// It can be used to make GET or DELETE requests (if Body is nil) or POST or PUT
// requests (if Body is not nil). In latter case the body will be serialized to
// JSON.
//
// Respects context's deadline and cancellation.
func (r *Request) Do(ctx context.Context) error {
	// Grab a client first. Use same client for all retry attempts.
	var client *http.Client
	if clientFactory != nil {
		var err error
		if client, err = clientFactory(ctx, r.Scopes); err != nil {
			return err
		}
	} else {
		client = http.DefaultClient
		if testTransport := ctx.Value(&testTransportKey); testTransport != nil {
			client = &http.Client{Transport: testTransport.(http.RoundTripper)}
		}
	}

	// Prepare a blob with the request body. Marshal it once, to avoid
	// remarshaling on retries.
	isJSON := false
	var bodyBlob []byte
	if blob, ok := r.Body.([]byte); ok {
		bodyBlob = blob
	} else if r.Body != nil {
		var err error
		if bodyBlob, err = json.Marshal(r.Body); err != nil {
			return err
		}
		isJSON = true
	}

	return fetchJSON(ctx, client, r.Out, func() (*http.Request, error) {
		req, err := http.NewRequest(r.Method, r.URL, bytes.NewReader(bodyBlob))
		if err != nil {
			return nil, err
		}
		for k, v := range r.Headers {
			req.Header.Set(k, v)
		}
		if isJSON {
			req.Header.Set("Content-Type", "application/json")
		}
		return req, nil
	})
}

// TODO(vadimsh): Add retries on HTTP 500.

// fetchJSON fetches JSON document by making a request using given client.
func fetchJSON(ctx context.Context, client *http.Client, val any, f func() (*http.Request, error)) error {
	r, err := f()
	if err != nil {
		logging.Errorf(ctx, "auth: URL fetch failed - %s", err)
		return err
	}
	logging.Infof(ctx, "auth: %s %s", r.Method, r.URL)
	resp, err := ctxhttp.Do(ctx, client, r)
	if err != nil {
		logging.Errorf(ctx, "auth: URL fetch failed, can't connect - %s", err)
		return transient.Tag.Apply(err)
	}
	defer func() {
		io.ReadAll(resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		// Opportunistically try to unmarshal the response. Works with JSON APIs.
		if val != nil {
			json.Unmarshal(body, val)
		}
		logging.Errorf(ctx, "auth: URL fetch failed - HTTP %d - %s", resp.StatusCode, string(body))
		err := fmt.Errorf("auth: HTTP code (%d) when fetching %s", resp.StatusCode, r.URL)
		if resp.StatusCode >= 500 {
			return transient.Tag.Apply(err)
		}
		return err
	}
	if val != nil {
		if err = json.NewDecoder(resp.Body).Decode(val); err != nil {
			logging.Errorf(ctx, "auth: URL fetch failed, bad JSON - %s", err)
			return fmt.Errorf("auth: can't deserialize JSON at %q - %s", r.URL, err)
		}
	}
	return nil
}
