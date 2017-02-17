// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package localauth

import (
	"testing"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/lucictx"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWithLocalAuth(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeLocal)

	srv := Server{
		TokenGenerator: func(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
			return &oauth2.Token{
				AccessToken: "tok",
				Expiry:      clock.Now(ctx).Add(30 * time.Minute),
			}, nil
		},
	}

	Convey("Works", t, func() {
		WithLocalAuth(ctx, &srv, func(ctx context.Context) error {
			p := lucictx.GetLocalAuth(ctx)
			So(p, ShouldNotBeNil)
			req := prepReq(p, "/rpc/LuciLocalAuthService.GetOAuthToken", map[string]interface{}{
				"scopes": []string{"B", "A"},
				"secret": p.Secret,
			})
			So(call(req), ShouldEqual, `HTTP 200 (json): {"access_token":"tok","expiry":1454502906}`)
			return nil
		})
	})
}
