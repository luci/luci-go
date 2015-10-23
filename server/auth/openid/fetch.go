// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package openid

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

type requestFactory func() (*http.Request, error)

// fetchJSON fetches JSON document by making a request using a transport in
// the context.
func fetchJSON(c context.Context, val interface{}, f requestFactory) error {
	r, err := f()
	if err != nil {
		logging.Errorf(c, "openid: URL fetch failed - %s", err)
		return err
	}
	logging.Infof(c, "openid: %s %s", r.Method, r.URL)
	resp, err := transport.GetClient(c).Do(r)
	if err != nil {
		logging.Errorf(c, "openid: URL fetch failed, can't connect - %s", err)
		return errors.WrapTransient(err)
	}
	defer func() {
		ioutil.ReadAll(resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode != 200 {
		body, _ := ioutil.ReadAll(resp.Body)
		logging.Errorf(c, "openid: URL fetch failed - HTTP %d - %s", resp.StatusCode, string(body))
		return errors.WrapTransient(
			fmt.Errorf("openid: unexpected HTTP code (%d) when fetching %s", resp.StatusCode, r.URL))
	}
	if err = json.NewDecoder(resp.Body).Decode(val); err != nil {
		logging.Errorf(c, "openid: URL fetch failed, bad JSON - %s", err)
		return fmt.Errorf("openid: can't deserialize JSON at %q - %s", r.URL, err)
	}
	return nil
}
