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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/lucictx"
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

	ftt.Run("With server", t, func(c *ftt.Test) {
		// Use channels to pass mocked requests/responses back and forth.
		requests := make(chan []string, 10000)
		responses := make(chan any, 1)

		testGen := func(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
			requests <- scopes
			var resp any
			select {
			case resp = <-responses:
			default:
				c.Log("Unexpected token request")
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
		assert.Loosely(c, err, should.BeNil)
		defer s.Stop(ctx)

		assert.Loosely(c, p.Accounts[0], should.Resemble(&lucictx.LocalAuthAccount{
			Id: "acc_id", Email: "some@example.com",
		}))
		assert.Loosely(c, p.Accounts[1], should.Resemble(&lucictx.LocalAuthAccount{
			Id: "another_id", Email: "another@example.com",
		}))
		assert.Loosely(c, p.DefaultAccountId, should.Equal("acc_id"))

		goodOAuthRequest := func() *http.Request {
			return prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]any{
				"scopes":     []string{"B", "A"},
				"secret":     p.Secret,
				"account_id": "acc_id",
			})
		}

		goodIDTokRequest := func() *http.Request {
			return prepReq(p, "/rpc/LuciLocalAuthService.GetIDToken", map[string]any{
				"audience":   "A",
				"secret":     p.Secret,
				"account_id": "acc_id",
			})
		}

		c.Run("Access tokens happy path", func(c *ftt.Test) {
			responses <- &oauth2.Token{
				AccessToken: "tok1",
				Expiry:      clock.Now(ctx).Add(30 * time.Minute),
			}
			assert.Loosely(c, call(goodOAuthRequest()), should.Equal(`HTTP 200 (json): {"access_token":"tok1","expiry":1454474106}`))
			assert.Loosely(c, <-requests, should.Resemble([]string{"A", "B"}))

			// application/json is also the default.
			req := goodOAuthRequest()
			req.Header.Del("Content-Type")
			responses <- &oauth2.Token{
				AccessToken: "tok2",
				Expiry:      clock.Now(ctx).Add(30 * time.Minute),
			}
			assert.Loosely(c, call(req), should.Equal(`HTTP 200 (json): {"access_token":"tok2","expiry":1454474106}`))
			assert.Loosely(c, <-requests, should.Resemble([]string{"A", "B"}))
		})

		c.Run("ID tokens happy path", func(c *ftt.Test) {
			responses <- &oauth2.Token{
				AccessToken: "tok1",
				Expiry:      clock.Now(ctx).Add(30 * time.Minute),
			}
			assert.Loosely(c, call(goodIDTokRequest()), should.Equal(`HTTP 200 (json): {"id_token":"tok1","expiry":1454474106}`))
			assert.Loosely(c, <-requests, should.Resemble([]string{"audience:A"}))

			// application/json is also the default.
			req := goodIDTokRequest()
			req.Header.Del("Content-Type")
			responses <- &oauth2.Token{
				AccessToken: "tok2",
				Expiry:      clock.Now(ctx).Add(30 * time.Minute),
			}
			assert.Loosely(c, call(req), should.Equal(`HTTP 200 (json): {"id_token":"tok2","expiry":1454474106}`))
			assert.Loosely(c, <-requests, should.Resemble([]string{"audience:A"}))
		})

		c.Run("Panic in token generator", func(c *ftt.Test) {
			responses <- "omg, panic"
			assert.Loosely(c, call(goodOAuthRequest()), should.Equal(`HTTP 500: Internal Server Error. See logs.`))
		})

		c.Run("Not POST", func(c *ftt.Test) {
			req := goodOAuthRequest()
			req.Method = "PUT"
			assert.Loosely(c, call(req), should.Equal(`HTTP 405: Expecting POST`))
		})

		c.Run("Bad URI", func(c *ftt.Test) {
			req := goodOAuthRequest()
			req.URL.Path = "/zzz"
			assert.Loosely(c, call(req), should.Equal(`HTTP 404: Expecting /rpc/LuciLocalAuthService.<method>`))
		})

		c.Run("Bad content type", func(c *ftt.Test) {
			req := goodOAuthRequest()
			req.Header.Set("Content-Type", "bzzzz")
			assert.Loosely(c, call(req), should.Equal(`HTTP 400: Expecting 'application/json' Content-Type`))
		})

		c.Run("Broken json", func(c *ftt.Test) {
			req := goodOAuthRequest()

			body := `not a json`
			req.Body = io.NopCloser(bytes.NewBufferString(body))
			req.ContentLength = int64(len(body))

			assert.Loosely(c, call(req), should.Equal(`HTTP 400: Not JSON body - invalid character 'o' in literal null (expecting 'u')`))
		})

		c.Run("Huge request", func(c *ftt.Test) {
			req := goodOAuthRequest()

			body := strings.Repeat("z", 64*1024+1)
			req.Body = io.NopCloser(bytes.NewBufferString(body))
			req.ContentLength = int64(len(body))

			assert.Loosely(c, call(req), should.Equal(`HTTP 400: Expecting 'Content-Length' header, <64Kb`))
		})

		c.Run("Unknown RPC method", func(c *ftt.Test) {
			req := prepReq(p, "/rpc/LuciLocalAuthService.UnknownMethod", map[string]any{})
			assert.Loosely(c, call(req), should.Equal(`HTTP 404: Unknown RPC method "UnknownMethod"`))
		})

		c.Run("No scopes", func(c *ftt.Test) {
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]any{
				"secret":     p.Secret,
				"account_id": "acc_id",
			})
			assert.Loosely(c, call(req), should.Equal(`HTTP 400: Bad request: field "scopes" is required.`))
		})

		c.Run("No audience", func(c *ftt.Test) {
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetIDToken", map[string]any{
				"secret":     p.Secret,
				"account_id": "acc_id",
			})
			assert.Loosely(c, call(req), should.Equal(`HTTP 400: Bad request: field "audience" is required.`))
		})

		c.Run("No secret", func(c *ftt.Test) {
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]any{
				"scopes":     []string{"B", "A"},
				"account_id": "acc_id",
			})
			assert.Loosely(c, call(req), should.Equal(`HTTP 400: Bad request: field "secret" is required.`))
		})

		c.Run("Bad secret", func(c *ftt.Test) {
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]any{
				"scopes":     []string{"B", "A"},
				"secret":     []byte{0, 1, 2, 3},
				"account_id": "acc_id",
			})
			assert.Loosely(c, call(req), should.Equal(`HTTP 403: Invalid secret.`))
		})

		c.Run("No account ID", func(c *ftt.Test) {
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]any{
				"scopes": []string{"B", "A"},
				"secret": p.Secret,
			})
			assert.Loosely(c, call(req), should.Equal(`HTTP 400: Bad request: field "account_id" is required.`))
		})

		c.Run("Unknown account ID", func(c *ftt.Test) {
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]any{
				"scopes":     []string{"B", "A"},
				"secret":     p.Secret,
				"account_id": "unknown_acc_id",
			})
			assert.Loosely(c, call(req), should.Equal(`HTTP 404: Unrecognized account ID "unknown_acc_id".`))
		})

		c.Run("Token generator returns fatal error", func(c *ftt.Test) {
			responses <- fmt.Errorf("fatal!!111")
			assert.Loosely(c, call(goodOAuthRequest()), should.Equal(`HTTP 200 (json): {"error_code":-1,"error_message":"fatal!!111"}`))
		})

		c.Run("Token generator returns ErrorWithCode", func(c *ftt.Test) {
			responses <- errWithCode{
				error: fmt.Errorf("with code"),
				code:  123,
			}
			assert.Loosely(c, call(goodOAuthRequest()), should.Equal(`HTTP 200 (json): {"error_code":123,"error_message":"with code"}`))
		})

		c.Run("Token generator returns transient error", func(c *ftt.Test) {
			responses <- errors.New("transient", transient.Tag)
			assert.Loosely(c, call(goodOAuthRequest()), should.Equal(`HTTP 500: Transient error - transient`))
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

func prepReq(p *lucictx.LocalAuth, uri string, body any) *http.Request {
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

func call(req *http.Request) any {
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
