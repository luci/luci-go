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

package authtest

import (
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/auth/localauth"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/lucictx"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFakeTokenGenerator(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

		srv := localauth.Server{
			TokenGenerators: map[string]localauth.TokenGenerator{
				"authtest": &FakeTokenGenerator{},
			},
			DefaultAccountID: "authtest",
		}
		la, err := srv.Start(ctx)
		So(err, ShouldBeNil)
		defer srv.Stop(ctx)

		auth := auth.NewAuthenticator(lucictx.SetLocalAuth(ctx, la), auth.SilentLogin, auth.Options{})

		email, err := auth.GetEmail()
		So(err, ShouldBeNil)
		So(email, ShouldEqual, DefaultFakeEmail)

		tok, err := auth.GetAccessToken(time.Minute)
		So(err, ShouldBeNil)
		So(tok.AccessToken, ShouldEqual, DefaultFakeToken)

		// Expiry is rounded to integer number of seconds, since that's the
		// granularity of OAuth token expiration. Compare int unix timestamps to
		// account for that.
		So(tok.Expiry.Unix(), ShouldEqual,
			testclock.TestRecentTimeUTC.Add(DefaultFakeLifetime).Unix())
	})
}
