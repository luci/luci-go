// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package internal

import (
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
		logging.Errorf(c, "auth: URL fetch failed - HTTP %d - %s", resp.StatusCode, string(body))
		err := fmt.Errorf("auth: unexpected HTTP code (%d) when fetching %s", resp.StatusCode, r.URL)
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
