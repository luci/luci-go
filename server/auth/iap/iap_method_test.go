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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"testing"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/gologger"
	"golang.org/x/net/context"
	"google.golang.org/api/idtoken"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIAPAuthenticator(t *testing.T) {
	t.Parallel()
	Convey("iap", t, func() {
		c := gaetesting.TestingContext()
		c = gologger.StdConfig.Use(c)
		c, _ = testclock.UseTime(c, time.Unix(1442540000, 0))

		Convey("missing iap jwt assertion header", func() {
			a := &IAPAuthMethod{}
			r := makeGetRequest()
			user, err := a.Authenticate(c, r)
			So(user, ShouldBeNil)
			So(err, ShouldEqual, ErrMissingJWTAssertionHeader)
		})

		Convey("invalid jwt assertion header bytes", func() {
			a := &IAPAuthMethod{}
			r := makeGetRequest()
			r.Header[IAPJWTAssertionHeader] = []string{"some invalid header bytes"}

			user, err := a.Authenticate(c, r)
			So(user, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})

		Convey("invalid wrong jwt audience", func() {
			var tp *idtoken.Payload

			a := &IAPAuthMethod{
				NumericProjectID: "1234",
				AppID:            "some-app-id",
				Validator: func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {
					return tp, nil
				},
			}
			r := makeGetRequest()
			jwtPayload := &idtoken.Payload{
				Issuer:   "",
				Audience: fmt.Sprintf("/projects/%s/apps/%s", a.NumericProjectID, a.AppID),
				Expires:  clock.Now(c).Add(2 * time.Hour).Unix(),
				IssuedAt: 0,
				Subject:  "",
				Claims: map[string]interface{}{
					"email": "someemail@somedomain.com",
				},
			}
			tp = jwtPayload
			payload, err := json.Marshal(jwtPayload)
			So(err, ShouldBeNil)
			header, err := json.Marshal(map[string]string{
				"alg": "ES256",
				"typ": "",
				"kid": "",
			})
			So(err, ShouldBeNil)

			signature := []byte("bogus signature")
			So(err, ShouldBeNil)

			jwtHeader := strings.Join([]string{
				base64.RawURLEncoding.EncodeToString(header),
				base64.RawURLEncoding.EncodeToString(payload),
				base64.RawURLEncoding.EncodeToString(signature),
			}, ".")
			r.Header[IAPJWTAssertionHeader] = []string{string(jwtHeader)}

			user, err := a.Authenticate(c, r)
			So(err, ShouldBeNil)
			So(user, ShouldNotBeNil)
		})

	})

}

func makeGetRequest() *http.Request {
	req, _ := http.NewRequest("GET", "/doesntmatter", nil)
	return req
}
