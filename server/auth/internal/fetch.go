// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/transport"
)

// TODO(vadimsh): Add retries on HTTP 500.

// RequestFactory is used by FetchJSON to create http.Requests. It may be called
// multiple times when FetchJSON retries the fetch.
type RequestFactory func() (*http.Request, error)

// FetchJSON fetches JSON document by making a request using a transport in
// the context.
func FetchJSON(c context.Context, val interface{}, f RequestFactory) error {
	r, err := f()
	if err != nil {
		logging.Errorf(c, "auth: URL fetch failed - %s", err)
		return err
	}
	logging.Infof(c, "auth: %s %s", r.Method, r.URL)
	resp, err := transport.GetClient(c).Do(r)
	if err != nil {
		logging.Errorf(c, "auth: URL fetch failed, can't connect - %s", err)
		return errors.WrapTransient(err)
	}
	defer func() {
		ioutil.ReadAll(resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode >= 300 {
		body, _ := ioutil.ReadAll(resp.Body)
		// Opportunistically try to unmarshal the response. Works with JSON APIs.
		if val != nil {
			json.Unmarshal(body, val)
		}
		logging.Errorf(c, "auth: URL fetch failed - HTTP %d - %s", resp.StatusCode, string(body))
		err := fmt.Errorf("auth: HTTP code (%d) when fetching %s", resp.StatusCode, r.URL)
		if resp.StatusCode >= 500 {
			return errors.WrapTransient(err)
		}
		return err
	}
	if val != nil {
		if err = json.NewDecoder(resp.Body).Decode(val); err != nil {
			logging.Errorf(c, "auth: URL fetch failed, bad JSON - %s", err)
			return fmt.Errorf("auth: can't deserialize JSON at %q - %s", r.URL, err)
		}
	}
	return nil
}

// GetJSON makes GET request.
func GetJSON(c context.Context, url string, out interface{}) error {
	return methodWithoutBody(c, "GET", url, out)
}

// DeleteJSON makes DELETE request.
func DeleteJSON(c context.Context, url string, out interface{}) error {
	return methodWithoutBody(c, "DELETE", url, out)
}

// PostJSON makes POST request.
func PostJSON(c context.Context, url string, body, out interface{}) error {
	return methodWithBody(c, "POST", url, body, out)
}

// PutJSON makes PUT request.
func PutJSON(c context.Context, url string, body, out interface{}) error {
	return methodWithBody(c, "PUT", url, body, out)
}

// methodWithoutBody makes request that doesn't have request body (e.g GET).
func methodWithoutBody(c context.Context, method, url string, out interface{}) error {
	return FetchJSON(c, out, func() (*http.Request, error) {
		return http.NewRequest(method, url, nil)
	})
}

// methodWithoutBody makes request that does have request body (e.g POST).
func methodWithBody(c context.Context, method, url string, body, out interface{}) error {
	var blob []byte
	if body != nil {
		var err error
		if blob, err = json.Marshal(body); err != nil {
			return err
		}
	}
	return FetchJSON(c, out, func() (*http.Request, error) {
		req, err := http.NewRequest(method, url, bytes.NewReader(blob))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
}
