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
	"testing"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/lucictx"

	. "github.com/smartystreets/goconvey/convey"
)

type callbackGen struct {
	email string
	cb    func(context.Context, []string, time.Duration) (*oauth2.Token, error)
}

func (g *callbackGen) GenerateToken(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
	return g.cb(ctx, scopes, lifetime)
}

func (g *callbackGen) GetEmail() (string, error) {
	return g.email, nil
}

func makeGenerator(email string, cb func(context.Context, []string, time.Duration) (*oauth2.Token, error)) TokenGenerator {
	return &callbackGen{email, cb}
}

func TestWithLocalAuth(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

	gen := makeGenerator("email@example.com", func(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
		return &oauth2.Token{
			AccessToken: "tok",
			Expiry:      clock.Now(ctx).Add(30 * time.Minute),
		}, nil
	})

	srv := Server{
		TokenGenerators: map[string]TokenGenerator{"acc_id": gen},
	}

	Convey("Works", t, func() {
		WithLocalAuth(ctx, &srv, func(ctx context.Context) error {
			p := lucictx.GetLocalAuth(ctx)
			So(p, ShouldNotBeNil)
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]interface{}{
				"scopes":     []string{"B", "A"},
				"secret":     p.Secret,
				"account_id": "acc_id",
			})
			So(call(req), ShouldEqual, `HTTP 200 (json): {"access_token":"tok","expiry":1454474106}`)
			return nil
		})
	})
}
