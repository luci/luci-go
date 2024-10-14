// Copyright 2017 The LUCI Authors.
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

package googleoauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGetTokenInfo(t *testing.T) {
	ftt.Run("Works", t, func(t *ftt.Test) {
		goodToken := true
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				panic("not a GET")
			}
			if r.RequestURI != "/?access_token=zzz%3D%3D%3Dtoken" {
				panic("wrong URI " + r.RequestURI)
			}
			if goodToken {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				w.Write([]byte(`{"aud":"blah", "expires_in":"12345", "email_verified": "true"}`))
			} else {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(400)
				w.Write([]byte(`{"error_description":"Invalid value"}`))
			}
		}))
		defer ts.Close()

		info, err := GetTokenInfo(context.Background(), TokenInfoParams{
			AccessToken: "zzz===token",
			Endpoint:    ts.URL,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, info, should.Resemble(&TokenInfo{
			Aud:           "blah",
			ExpiresIn:     12345,
			EmailVerified: true,
		}))

		goodToken = false
		info, err = GetTokenInfo(context.Background(), TokenInfoParams{
			AccessToken: "zzz===token",
			Endpoint:    ts.URL,
		})
		assert.Loosely(t, info, should.BeNil)
		assert.Loosely(t, err, should.Equal(ErrBadToken))
	})
}
