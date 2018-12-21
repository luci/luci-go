// Copyright 2018 The LUCI Authors.
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

package projectscope

import (
	//"context"

	//"golang.org/x/oauth2"
	//. "github.com/smartystreets/goconvey/convey"

	//"go.chromium.org/luci/server/auth"
	//"go.chromium.org/luci/tokenserver/api/minter/v1"
	//"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectscope"
	"testing"
)

func TestMintServiceOAuthToken(t *testing.T) {
	/*
		ctx := testingContext("service:some-service")

		rules, _ := loadConfig(ctx, `rules {
			name: "rule 1"
			service_account: "serviceaccount@robots.com"
			proxy: "user:proxy@example.com"
			end_user: "user:enduser@example.com"
			allowed_scope: "https://www.googleapis.com/scope1"
			allowed_scope: "https://www.googleapis.com/scope2"
			max_grant_validity_duration: 7200
		}`)

		loggedInfo := &MintedServiceOAuthTokenInfo{}

		rpc := MintServiceOAuthTokenRPC{
			Signer: testingSigner(),
			Rules: func(context.Context) (*Rules, error) {
				return rules, nil
			},
			MintAccessToken: func(ctx context.Context, params auth.MintAccessTokenParams) (*oauth2.Token, error) {
				return nil, nil
			},
			LogOAuthToken: func(c context.Context, i LoggableOAuthTokenInfo) error {
				loggedInfo = i.(*MintedServiceOAuthTokenInfo)
				return nil
			},
			Storage: projectscope.Get(),
		}

		Convey("Happy path", t, func() {

			req := &minter.MintServiceOAuthTokenRequest{

			}

			resp, err := rpc.MintServiceOAuthToken(ctx, req)
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &minter.MintServiceOAuthTokenResponse{

			})
			So(loggedInfo, ShouldResemble, &MintedServiceOAuthTokenInfo{

			})

		})
	*/
}
