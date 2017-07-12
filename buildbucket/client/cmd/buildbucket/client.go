// Copyright 2016 The LUCI Authors.
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

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
	"github.com/luci/luci-go/common/retry/transient"
	"golang.org/x/net/context/ctxhttp"
)

type client struct {
	HTTP    *http.Client
	baseURL *url.URL
}

// call makes an HTTP call with exponential back-off on transient errors.
// if urlStr is relative, rebases it on c.baseURL.
func (c *client) call(ctx context.Context, method, urlStr string, body interface{}) (response []byte, err error) {
	if !c.baseURL.IsAbs() {
		panic("baseURL is not absolute")
	}

	var bodyBytes []byte
	if body != nil {
		var err error
		if bodyBytes, err = json.Marshal(body); err != nil {
			return nil, err
		}
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid urlStr: %s", err)
	}
	u = c.baseURL.ResolveReference(u)
	logging.Infof(ctx, "%s %s", method, u)

	err = retry.Retry(
		ctx,
		transient.Only(retry.Default),
		func() error {
			req := &http.Request{Method: method, URL: u}
			if bodyBytes != nil {
				req.Body = ioutil.NopCloser(bytes.NewReader(bodyBytes))
			}
			res, err := ctxhttp.Do(ctx, c.HTTP, req)
			if err != nil {
				return transient.Tag.Apply(err)
			}

			defer res.Body.Close()
			if res.StatusCode >= 500 {
				bodyBytes, _ := ioutil.ReadAll(res.Body)
				return transient.Tag.Apply(fmt.Errorf("status %s: %s", res.Status, bodyBytes))
			}

			response, err = ioutil.ReadAll(res.Body)
			return err
		},
		func(err error, wait time.Duration) {
			logging.WithError(err).Warningf(ctx, "API request failed transiently, will retry in %s", wait)
		},
	)
	return
}
