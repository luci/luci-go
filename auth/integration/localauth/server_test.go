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

package localauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/lucictx"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type callbackGen struct {
	email string
	cb    func(context.Context, []string, time.Duration) (*oauth2.Token, error)
}

func (g *callbackGen) GenerateOAuthToken(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
	return g.cb(ctx, scopes, lifetime)
}

func (g *callbackGen) GenerateIDToken(ctx context.Context, audience string, lifetime time.Duration) (*oauth2.Token, error) {
	return g.cb(ctx, []string{"audience:" + audience}, lifetime)
}

func (g *callbackGen) GetEmail() (string, error) {
	return g.email, nil
}

func makeGenerator(email string, cb func(context.Context, []string, time.Duration) (*oauth2.Token, error)) TokenGenerator {
	return &callbackGen{email, cb}
}

func TestProtocol(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

	Convey("With server", t, func(c C) {
		// Use channels to pass mocked requests/responses back and forth.
		requests := make(chan []string, 10000)
		responses := make(chan interface{}, 1)

		testGen := func(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
			requests <- scopes
			var resp interface{}
			select {
			case resp = <-responses:
			default:
				c.Println("Unexpected token request")
				return nil, fmt.Errorf("Unexpected request")
			}
			switch resp := resp.(type) {
			case error:
				return nil, resp
			case *oauth2.Token:
				return resp, nil
			default:
				panic("unknown response")
			}
		}

		s := Server{
			TokenGenerators: map[string]TokenGenerator{
				"acc_id":     makeGenerator("some@example.com", testGen),
				"another_id": makeGenerator("another@example.com", testGen),
			},
			DefaultAccountID: "acc_id",
		}
		p, err := s.Start(ctx)
		So(err, ShouldBeNil)
		defer s.Stop(ctx)

		So(p.Accounts[0], ShouldResembleProto, &lucictx.LocalAuthAccount{
			Id: "acc_id", Email: "some@example.com",
		})
		So(p.Accounts[1], ShouldResembleProto, &lucictx.LocalAuthAccount{
			Id: "another_id", Email: "another@example.com",
		})
		So(p.DefaultAccountId, ShouldEqual, "acc_id")

		goodOAuthRequest := func() *http.Request {
			return prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]interface{}{
				"scopes":     []string{"B", "A"},
				"secret":     p.Secret,
				"account_id": "acc_id",
			})
		}

		goodIDTokRequest := func() *http.Request {
			return prepReq(p, "/rpc/LuciLocalAuthService.GetIDToken", map[string]interface{}{
				"audience":   "A",
				"secret":     p.Secret,
				"account_id": "acc_id",
			})
		}

		Convey("Access tokens happy path", func() {
			responses <- &oauth2.Token{
				AccessToken: "tok1",
				Expiry:      clock.Now(ctx).Add(30 * time.Minute),
			}
			So(call(goodOAuthRequest()), ShouldEqual, `HTTP 200 (json): {"access_token":"tok1","expiry":1454474106}`)
			So(<-requests, ShouldResemble, []string{"A", "B"})

			// application/json is also the default.
			req := goodOAuthRequest()
			req.Header.Del("Content-Type")
			responses <- &oauth2.Token{
				AccessToken: "tok2",
				Expiry:      clock.Now(ctx).Add(30 * time.Minute),
			}
			So(call(req), ShouldEqual, `HTTP 200 (json): {"access_token":"tok2","expiry":1454474106}`)
			So(<-requests, ShouldResemble, []string{"A", "B"})
		})

		Convey("ID tokens happy path", func() {
			responses <- &oauth2.Token{
				AccessToken: "tok1",
				Expiry:      clock.Now(ctx).Add(30 * time.Minute),
			}
			So(call(goodIDTokRequest()), ShouldEqual, `HTTP 200 (json): {"id_token":"tok1","expiry":1454474106}`)
			So(<-requests, ShouldResemble, []string{"audience:A"})

			// application/json is also the default.
			req := goodIDTokRequest()
			req.Header.Del("Content-Type")
			responses <- &oauth2.Token{
				AccessToken: "tok2",
				Expiry:      clock.Now(ctx).Add(30 * time.Minute),
			}
			So(call(req), ShouldEqual, `HTTP 200 (json): {"id_token":"tok2","expiry":1454474106}`)
			So(<-requests, ShouldResemble, []string{"audience:A"})
		})

		Convey("Panic in token generator", func() {
			responses <- "omg, panic"
			So(call(goodOAuthRequest()), ShouldEqual, `HTTP 500: Internal Server Error. See logs.`)
		})

		Convey("Not POST", func() {
			req := goodOAuthRequest()
			req.Method = "PUT"
			So(call(req), ShouldEqual, `HTTP 405: Expecting POST`)
		})

		Convey("Bad URI", func() {
			req := goodOAuthRequest()
			req.URL.Path = "/zzz"
			So(call(req), ShouldEqual, `HTTP 404: Expecting /rpc/LuciLocalAuthService.<method>`)
		})

		Convey("Bad content type", func() {
			req := goodOAuthRequest()
			req.Header.Set("Content-Type", "bzzzz")
			So(call(req), ShouldEqual, `HTTP 400: Expecting 'application/json' Content-Type`)
		})

		Convey("Broken json", func() {
			req := goodOAuthRequest()

			body := `not a json`
			req.Body = io.NopCloser(bytes.NewBufferString(body))
			req.ContentLength = int64(len(body))

			So(call(req), ShouldEqual, `HTTP 400: Not JSON body - invalid character 'o' in literal null (expecting 'u')`)
		})

		Convey("Huge request", func() {
			req := goodOAuthRequest()

			body := strings.Repeat("z", 64*1024+1)
			req.Body = io.NopCloser(bytes.NewBufferString(body))
			req.ContentLength = int64(len(body))

			So(call(req), ShouldEqual, `HTTP 400: Expecting 'Content-Length' header, <64Kb`)
		})

		Convey("Unknown RPC method", func() {
			req := prepReq(p, "/rpc/LuciLocalAuthService.UnknownMethod", map[string]interface{}{})
			So(call(req), ShouldEqual, `HTTP 404: Unknown RPC method "UnknownMethod"`)
		})

		Convey("No scopes", func() {
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]interface{}{
				"secret":     p.Secret,
				"account_id": "acc_id",
			})
			So(call(req), ShouldEqual, `HTTP 400: Bad request: field "scopes" is required.`)
		})

		Convey("No audience", func() {
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetIDToken", map[string]interface{}{
				"secret":     p.Secret,
				"account_id": "acc_id",
			})
			So(call(req), ShouldEqual, `HTTP 400: Bad request: field "audience" is required.`)
		})

		Convey("No secret", func() {
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]interface{}{
				"scopes":     []string{"B", "A"},
				"account_id": "acc_id",
			})
			So(call(req), ShouldEqual, `HTTP 400: Bad request: field "secret" is required.`)
		})

		Convey("Bad secret", func() {
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]interface{}{
				"scopes":     []string{"B", "A"},
				"secret":     []byte{0, 1, 2, 3},
				"account_id": "acc_id",
			})
			So(call(req), ShouldEqual, `HTTP 403: Invalid secret.`)
		})

		Convey("No account ID", func() {
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]interface{}{
				"scopes": []string{"B", "A"},
				"secret": p.Secret,
			})
			So(call(req), ShouldEqual, `HTTP 400: Bad request: field "account_id" is required.`)
		})

		Convey("Unknown account ID", func() {
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]interface{}{
				"scopes":     []string{"B", "A"},
				"secret":     p.Secret,
				"account_id": "unknown_acc_id",
			})
			So(call(req), ShouldEqual, `HTTP 404: Unrecognized account ID "unknown_acc_id".`)
		})

		Convey("Token generator returns fatal error", func() {
			responses <- fmt.Errorf("fatal!!111")
			So(call(goodOAuthRequest()), ShouldEqual, `HTTP 200 (json): {"error_code":-1,"error_message":"fatal!!111"}`)
		})

		Convey("Token generator returns ErrorWithCode", func() {
			responses <- errWithCode{
				error: fmt.Errorf("with code"),
				code:  123,
			}
			So(call(goodOAuthRequest()), ShouldEqual, `HTTP 200 (json): {"error_code":123,"error_message":"with code"}`)
		})

		Convey("Token generator returns transient error", func() {
			responses <- errors.New("transient", transient.Tag)
			So(call(goodOAuthRequest()), ShouldEqual, `HTTP 500: Transient error - transient`)
		})
	})
}

type errWithCode struct {
	error
	code int
}

func (e errWithCode) Code() int {
	return e.code
}

func prepReq(p *lucictx.LocalAuth, uri string, body interface{}) *http.Request {
	var reader io.Reader
	isJSON := false
	if body != nil {
		blob, ok := body.([]byte)
		if !ok {
			var err error
			blob, err = json.Marshal(body)
			if err != nil {
				panic(err)
			}
			isJSON = true
		}
		reader = bytes.NewReader(blob)
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d%s", p.RpcPort, uri), reader)
	if err != nil {
		panic(err)
	}
	if isJSON {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func call(req *http.Request) interface{} {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	blob, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	tp := ""
	if resp.Header.Get("Content-Type") == "application/json; charset=utf-8" {
		tp = " (json)"
	}

	return fmt.Sprintf("HTTP %d%s: %s", resp.StatusCode, tp, strings.TrimSpace(string(blob)))
}
