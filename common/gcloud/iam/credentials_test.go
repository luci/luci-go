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

	. "github.com/smartystreets/goconvey/convey"
)

func TestCredentialsClient(t *testing.T) {
	t.Parallel()

	Convey("SignBlob works", t, func(c C) {
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
		So(err, ShouldBeNil)
		So(keyID, ShouldEqual, "key_id")
		So(string(sig), ShouldEqual, "signature")

		// The request body looks sane too.
		body := <-bodies
		So(string(body), ShouldEqual, `{"payload":"YmxvYg=="}`)
	})

	Convey("SignJWT works", t, func(c C) {
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
		So(err, ShouldBeNil)
		So(keyID, ShouldEqual, "key_id")
		So(jwt, ShouldEqual, "signed_jwt")

		// The request body looks sane too.
		body := <-bodies
		So(string(body), ShouldEqual, `{"payload":"{\"iss\":\"\",\"aud\":\"\",\"exp\":123,\"iat\":0}"}`)
	})

	Convey("GenerateAccessToken works", t, func(c C) {
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
					c.Printf("Bad body: %s\n", err)
					w.WriteHeader(500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				resp := fmt.Sprintf(`{"accessToken":"%s","expireTime":"%s"}`, "token1", expireTime.Format(time.RFC3339))
				w.Write([]byte(resp))

			default:
				c.Printf("Unknown URL: %q\n", r.URL.Path)
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
		So(err, ShouldBeNil)
		So(token.AccessToken, ShouldEqual, "token1")
		So(token.Expiry, ShouldResemble, expireTime)

		So(body, ShouldResemble, map[string]any{
			"delegates": []any{"deleg"},
			"scope":     []any{"a", "b"},
			"lifetime":  "30m0s",
		})
	})

	Convey("GenerateIDToken works", t, func(c C) {
		var body map[string]any

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			switch r.URL.Path {
			case "/v1/projects/-/serviceAccounts/abc@example.com:generateIdToken":
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					c.Printf("Bad body: %s\n", err)
					w.WriteHeader(500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				w.Write([]byte(`{"token":"fake_id_token"}`))

			default:
				c.Printf("Unknown URL: %q\n", r.URL.Path)
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
		So(err, ShouldBeNil)
		So(token, ShouldEqual, "fake_id_token")

		So(body, ShouldResemble, map[string]any{
			"delegates":    []any{"deleg"},
			"audience":     "aud",
			"includeEmail": true,
		})
	})
}
