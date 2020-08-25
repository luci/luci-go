// Copyright 2020 The LUCI Authors.
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

package iap

import (
	"net/http"
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/api/idtoken"

	"go.chromium.org/luci/common/logging/gologger"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIAPAuthenticator(t *testing.T) {
	t.Parallel()
	Convey("iap", t, func() {
		c := context.Background()
		c = gologger.StdConfig.Use(c)

		Convey("missing iap jwt assertion header", func() {
			a := &IAPAuthMethod{}
			r := makeGetRequest()
			user, err := a.Authenticate(c, r)
			So(user, ShouldBeNil)
			So(err, ShouldBeNil)
		})

		Convey("invalid jwt assertion header bytes", func() {
			a := &IAPAuthMethod{}
			r := makeGetRequest()
			r.Header[iapJWTAssertionHeader] = []string{"some invalid header bytes"}

			user, err := a.Authenticate(c, r)
			So(user, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})

		Convey("invalid multiple assertion header values", func() {
			a := &IAPAuthMethod{}
			r := makeGetRequest()
			r.Header[iapJWTAssertionHeader] = []string{"only", "expect", "one", "header", "value"}

			user, err := a.Authenticate(c, r)
			So(user, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})

		Convey("invalid no email claims", func() {
			a := &IAPAuthMethod{
				Aud: AudForGAE("1234", "some-app-id"),
				validator: func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {
					return &idtoken.Payload{
						Issuer:   "",
						IssuedAt: 0,
						Subject:  "",
					}, nil
				},
			}
			r := makeGetRequest()
			r.Header[iapJWTAssertionHeader] = []string{"just needs to be non-empty for testing"}

			user, err := a.Authenticate(c, r)
			So(user, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})

		Convey("happy path", func() {
			a := &IAPAuthMethod{
				Aud: AudForGAE("1234", "some-app-id"),
				validator: func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {
					return &idtoken.Payload{
						Issuer:   "",
						IssuedAt: 0,
						Subject:  "",
						Claims: map[string]interface{}{
							"email": "someemail@somedomain.com",
						},
					}, nil
				},
			}

			r := makeGetRequest()
			r.Header[iapJWTAssertionHeader] = []string{string("just needs to be non-empty for testing")}

			user, err := a.Authenticate(c, r)
			So(err, ShouldBeNil)
			So(user, ShouldNotBeNil)
			So(user.Email, ShouldEqual, "someemail@somedomain.com")
		})

	})

}

func makeGetRequest() *http.Request {
	req, _ := http.NewRequest("GET", "/doesntmatter", nil)
	return req
}
