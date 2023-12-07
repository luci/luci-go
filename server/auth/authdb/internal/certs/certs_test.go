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

package certs

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/server/auth/internal"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWorks(t *testing.T) {
	Convey("GetCertificates works", t, func(c C) {
		tokenService := signingtest.NewSigner(&signing.ServiceInfo{
			AppID:              "token-server",
			ServiceAccountName: "token-server-account@example.com",
		})

		ctx, tc := testclock.UseTime(context.Background(), time.Time{})
		ctx = caching.WithEmptyProcessCache(ctx)

		calls := 0

		ctx = internal.WithTestTransport(ctx, func(r *http.Request, body string) (int, string) {
			calls++
			if r.URL.String() != "http://token-server/auth/api/v1/server/certificates" {
				return 404, "Wrong URL"
			}
			certs, err := tokenService.Certificates(ctx)
			if err != nil {
				panic(err)
			}
			blob, err := json.Marshal(certs)
			if err != nil {
				panic(err)
			}
			return 200, string(blob)
		})

		bundle := Bundle{ServiceURL: "http://token-server"}

		id, certs, err := bundle.GetCerts(ctx)
		So(err, ShouldBeNil)
		So(id, ShouldEqual, identity.Identity("user:token-server-account@example.com"))
		So(certs, ShouldNotBeNil)
		So(calls, ShouldEqual, 1)

		// Reuses stuff from cache.
		id, certs, err = bundle.GetCerts(ctx)
		So(err, ShouldBeNil)
		So(id, ShouldEqual, identity.Identity("user:token-server-account@example.com"))
		So(certs, ShouldNotBeNil)
		So(calls, ShouldEqual, 1)

		tc.Add(time.Hour + 5*time.Minute)

		// Until it expires.
		id, certs, err = bundle.GetCerts(ctx)
		So(err, ShouldBeNil)
		So(id, ShouldEqual, identity.Identity("user:token-server-account@example.com"))
		So(certs, ShouldNotBeNil)
		So(calls, ShouldEqual, 2)
	})
}
