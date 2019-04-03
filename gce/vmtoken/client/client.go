// Copyright 2019 The LUCI Authors.
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

// Package client implements client-side fetch and transmission of signed GCE VM
// metadata tokens.
package client

import (
	"fmt"
	"net/http"
	"net/url"

	"cloud.google.com/go/compute/metadata"

	"go.chromium.org/luci/auth"

	"go.chromium.org/luci/gce/vmtoken"
)

// NewClient returns an *http.Client which sets a GCE VM metadata token in the
// Header of an *http.Request during Do. The audience for the token is the
// *http.Request.URL.Host.
func NewClient(meta *metadata.Client, acc string) *http.Client {
	return &http.Client{
		Transport: newRoundTripper(meta, acc),
	}
}

func newRoundTripper(meta *metadata.Client, acc string) http.RoundTripper {
	return auth.NewModifyingTransport(http.DefaultTransport, newTokenInjector(meta, acc))
}

// tokMetadata is the metadata path which returns VM tokens.
const tokMetadata = "instance/service-accounts/%s/identity?audience=%s&format=full"

// newTokenInjector returns a function which can modify the Header of an
// *http.Request to include a GCE VM metadata token for the given account.
func newTokenInjector(meta *metadata.Client, acc string) func(*http.Request) error {
	if acc == "" {
		acc = "default"
	}
	acc = url.PathEscape(acc)
	return func(req *http.Request) error {
		aud := fmt.Sprintf("%s://%s", req.URL.Scheme, req.URL.Host)
		aud = url.QueryEscape(aud)
		// TODO(smut): Cache the token and reuse if not yet expired.
		// Currently the only user of this package only makes one
		// request per boot so caching isn't too important yet.
		tok, err := meta.Get(fmt.Sprintf(tokMetadata, acc, aud))
		if err != nil {
			return err
		}
		req.Header.Set(vmtoken.Header, tok)
		return nil
	}
}
