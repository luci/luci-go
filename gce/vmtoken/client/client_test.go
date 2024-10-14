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

package client

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"cloud.google.com/go/compute/metadata"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gce/appengine/testing/roundtripper"
	"go.chromium.org/luci/gce/vmtoken"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	ftt.Run("NewClient", t, func(c *ftt.Test) {
		// Create a test server which expects the token.
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			assert.Loosely(c, req.Header.Get(vmtoken.Header), should.Equal("token"))
		}))

		// Create a mock metadata client which returns the token.
		meta := metadata.NewClient(&http.Client{
			Transport: &roundtripper.StringRoundTripper{
				Handler: func(req *http.Request) (int, string) {
					assert.Loosely(c, req.URL.Path, should.Equal("/computeMetadata/v1/instance/service-accounts/account/identity"))
					url, err := url.Parse(srv.URL)
					assert.Loosely(c, err, should.BeNil)
					assert.Loosely(c, req.URL.Query().Get("audience"), should.Equal("http://"+url.Host))
					return http.StatusOK, "token"
				},
			},
		})

		cli := NewClient(meta, "account")
		_, err := cli.Get(srv.URL)
		assert.Loosely(c, err, should.BeNil)
	})
}
