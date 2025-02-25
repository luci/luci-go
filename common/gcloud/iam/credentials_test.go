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

package iam

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCredentialsClient(t *testing.T) {
	t.Parallel()

	ftt.Run("SignBlob works", t, func(c *ftt.Test) {
		bodies := make(chan []byte, 1)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.URL.Path != "/v1/projects/-/serviceAccounts/abc@example.com:signBlob" {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				panic(err)
			}
			bodies <- body

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(
				fmt.Sprintf(`{"keyId":"key_id","signedBlob":"%s"}`,
					base64.StdEncoding.EncodeToString([]byte("signature")))))
		}))
		defer ts.Close()

		cl := CredentialsClient{
			Client:     http.DefaultClient,
			backendURL: ts.URL,
		}

		keyID, sig, err := cl.SignBlob(context.Background(), "abc@example.com", []byte("blob"))
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, keyID, should.Equal("key_id"))
		assert.Loosely(c, string(sig), should.Equal("signature"))

		// The request body looks sane too.
		body := <-bodies
		assert.Loosely(c, string(body), should.Equal(`{"payload":"YmxvYg=="}`))
	})

	ftt.Run("SignJWT works", t, func(c *ftt.Test) {
		bodies := make(chan []byte, 1)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.URL.Path != "/v1/projects/-/serviceAccounts/abc@example.com:signJwt" {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				panic(err)
			}
			bodies <- body

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(`{"keyId":"key_id","signedJwt":"signed_jwt"}`))
		}))
		defer ts.Close()

		cl := CredentialsClient{
			Client:     http.DefaultClient,
			backendURL: ts.URL,
		}

		keyID, jwt, err := cl.SignJWT(context.Background(), "abc@example.com", &ClaimSet{Exp: 123})
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, keyID, should.Equal("key_id"))
		assert.Loosely(c, jwt, should.Equal("signed_jwt"))

		// The request body looks sane too.
		body := <-bodies
		assert.Loosely(c, string(body), should.Equal(`{"payload":"{\"iss\":\"\",\"aud\":\"\",\"exp\":123,\"iat\":0}"}`))
	})

	ftt.Run("GenerateAccessToken works", t, func(c *ftt.Test) {
		expireTime := testclock.TestRecentTimeUTC.Round(time.Second)

		var body map[string]any

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			switch r.URL.Path {
			case "/v1/projects/-/serviceAccounts/abc@example.com:generateAccessToken":
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					c.Logf("Bad body: %s\n", err)
					w.WriteHeader(500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				resp := fmt.Sprintf(`{"accessToken":"%s","expireTime":"%s"}`, "token1", expireTime.Format(time.RFC3339))
				w.Write([]byte(resp))

			default:
				c.Logf("Unknown URL: %q\n", r.URL.Path)
				w.WriteHeader(404)
			}
		}))
		defer ts.Close()

		cl := CredentialsClient{
			Client:     http.DefaultClient,
			backendURL: ts.URL,
		}

		token, err := cl.GenerateAccessToken(context.Background(),
			"abc@example.com", []string{"a", "b"}, []string{"deleg"}, 30*time.Minute)
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, token.AccessToken, should.Equal("token1"))
		assert.Loosely(c, token.Expiry, should.Match(expireTime))

		assert.Loosely(c, body, should.Match(map[string]any{
			"delegates": []any{"deleg"},
			"scope":     []any{"a", "b"},
			"lifetime":  "30m0s",
		}))
	})

	ftt.Run("GenerateIDToken works", t, func(c *ftt.Test) {
		var body map[string]any

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			switch r.URL.Path {
			case "/v1/projects/-/serviceAccounts/abc@example.com:generateIdToken":
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					c.Logf("Bad body: %s\n", err)
					w.WriteHeader(500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				w.Write([]byte(`{"token":"fake_id_token"}`))

			default:
				c.Logf("Unknown URL: %q\n", r.URL.Path)
				w.WriteHeader(404)
			}
		}))
		defer ts.Close()

		cl := CredentialsClient{
			Client:     http.DefaultClient,
			backendURL: ts.URL,
		}

		token, err := cl.GenerateIDToken(context.Background(),
			"abc@example.com", "aud", true, []string{"deleg"})
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, token, should.Equal("fake_id_token"))

		assert.Loosely(c, body, should.Match(map[string]any{
			"delegates":    []any{"deleg"},
			"audience":     "aud",
			"includeEmail": true,
		}))
	})
}
