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
	"fmt"
	"go.chromium.org/luci/common/retry/transient"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/gcloud/iam"
	"go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing/signingtest"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/serviceaccounts"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectscope"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
)

func testLogOAuthToken(_ context.Context, _ serviceaccounts.LoggableOAuthTokenInfo) error {
	return nil
}

func testMintAccessToken(ctx context.Context, params auth.MintAccessTokenParams) (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken:  "",
		TokenType:    "",
		RefreshToken: "",
		Expiry:       time.Now(),
	}, nil
}

func testMintAccessTokenWithError(ctx context.Context, params auth.MintAccessTokenParams) (*oauth2.Token, error) {
	return nil, fmt.Errorf("Intended error for testing")
}

func testCreateServiceAccount(ctx context.Context, gcpProject string, identity *projectscope.ScopedIdentity, client projectscope.ServiceAccountClient) (*iam.ServiceAccount, bool, error) {
	return &iam.ServiceAccount{
		Id:             "",
		Name:           "",
		ProjectId:      "",
		UniqueId:       "",
		Email:          "",
		DisplayName:    "",
		Oauth2ClientId: "",
	}, true, nil
}

func newTestMintServiceOAuthTokenRPC() *MintServiceOAuthTokenRPC {
	rpc := MintServiceOAuthTokenRPC{
		Signer: signingtest.NewSigner(nil),
		Rules: func(ctx context.Context) (*Rules, error) {
			return &Rules{
				revision: testingRevision,
				rulesPerService: map[string]*Rule{
					scopedServiceName: {
						CloudProjectID: scopedServiceGcpProject,
						Rule: &admin.ServiceConfig{
							Service:             scopedServiceName,
							CloudProjectId:      scopedServiceGcpProject,
							MaxValidityDuration: 3600,
						},
						Revision: testingRevision,
					},
					doubleCreateServiceName: {
						CloudProjectID: doubleCreateServiceGcpProject,
						Rule: &admin.ServiceConfig{
							Service:        doubleCreateServiceName,
							CloudProjectId: doubleCreateServiceGcpProject,
						},
						Revision: testingRevision,
					},
					"service@example.com": {
						CloudProjectID: "service-gcp-project",
						Rule: &admin.ServiceConfig{
							Service:             "service@example.com",
							CloudProjectId:      "service-gcp-project",
							MaxValidityDuration: 3600,
						},
						Revision: testingRevision,
					},
				},
			}, nil
		},
		MintAccessToken:      testMintAccessToken,
		LogOAuthToken:        testLogOAuthToken,
		Storage:              projectscope.ScopedIdentities,
		CreateServiceAccount: testCreateServiceAccount,
	}
	return &rpc
}

func TestMintServiceOAuthToken(t *testing.T) {

	t.Parallel()

	Convey("initialize rpc handler", t, func() {
		ctx := gaetesting.TestingContext()
		rpc := newTestMintServiceOAuthTokenRPC()

		Convey("validateRequest works", func() {

			Convey("empty fields", func() {
				req := &minter.MintServiceOAuthTokenRequest{
					LuciProject:         "",
					OauthScope:          []string{},
					MinValidityDuration: 7200,
				}
				rule, err := rpc.validateRequest(ctx, req, scopedServiceName)
				So(err, ShouldNotBeNil)
				So(rule, ShouldBeNil)
			})

			Convey("empty project", func() {

				req := &minter.MintServiceOAuthTokenRequest{
					LuciProject:         "",
					OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
					MinValidityDuration: 1800,
				}
				rule, err := rpc.validateRequest(ctx, req, "")
				So(err, assertions.ShouldErrLike, `luci project must be specified`)
				So(rule, ShouldBeNil)
			})

			Convey("empty scopes", func() {

				req := &minter.MintServiceOAuthTokenRequest{
					LuciProject:         "foo-project",
					OauthScope:          []string{},
					MinValidityDuration: 1800,
				}
				rule, err := rpc.validateRequest(ctx, req, "")
				So(err, assertions.ShouldErrLike, `oauth scopes must be specified`)
				So(rule, ShouldBeNil)
			})

			Convey("no rule for service", func() {

				req := &minter.MintServiceOAuthTokenRequest{
					LuciProject:         "test-project",
					OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
					MinValidityDuration: 3600,
				}
				rule, err := rpc.validateRequest(ctx, req, "some-unknown-caller")
				So(err, assertions.ShouldErrLike, `service some-unknown-caller not registered`)
				So(rule, ShouldBeNil)
			})

			Convey("returns valid and correct rule", func() {
				req := &minter.MintServiceOAuthTokenRequest{
					LuciProject:         "test-project",
					OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
					MinValidityDuration: 3600,
				}
				rule, err := rpc.validateRequest(ctx, req, scopedServiceName)
				So(err, ShouldBeNil)
				So(rule, ShouldResemble, &Rule{
					CloudProjectID: scopedServiceGcpProject,
					Rule: &admin.ServiceConfig{
						Service:             scopedServiceName,
						CloudProjectId:      scopedServiceGcpProject,
						MaxValidityDuration: 3600,
					},
					Revision: testingRevision,
				})
			})
		})

		Convey("checkRules works", func() {
			Convey("fail if service is not registered", func() {
				req := &minter.MintServiceOAuthTokenRequest{
					LuciProject:         "foo-project",
					OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
					MinValidityDuration: 7200,
				}
				resp, err := rpc.checkRules(ctx, req, scopedServiceName)
				So(err, assertions.ShouldErrLike, "the validity duration should be <=")
				So(resp, ShouldBeNil)
			})

			Convey("fail if requested validity duration is out of bounds", func() {
				req := &minter.MintServiceOAuthTokenRequest{
					LuciProject:         "",
					OauthScope:          []string{},
					MinValidityDuration: 0,
				}
				resp, err := rpc.checkRules(ctx, req, "")
				So(err, ShouldNotBeNil)
				So(resp, ShouldBeNil)
			})

			Convey("returns valid and correct rule", func() {
				req := &minter.MintServiceOAuthTokenRequest{
					LuciProject:         "foo-project",
					OauthScope:          []string{"https://www.googleapis.com/auth/cloud-platform"},
					MinValidityDuration: 1800,
				}
				resp, err := rpc.checkRules(ctx, req, scopedServiceName)
				So(err, ShouldBeNil)
				So(resp, ShouldNotBeNil)
			})
		})

		Convey("MintServiceOAuthToken does not return errors with valid input", func() {

			ctx := testingContext("service@example.com")
			req := &minter.MintServiceOAuthTokenRequest{
				LuciProject: "service-project",
				OauthScope:  []string{"https://www.googleapis.com/auth/cloud-platform"},
			}
			resp, err := rpc.MintServiceOAuthToken(ctx, req)
			So(err, ShouldBeNil)
			So(resp, ShouldNotBeNil)

		})

	})

	Convey("initialize rpc handler with service account creation error setup", t, func() {
		rpc := newTestMintServiceOAuthTokenRPC()
		rpc.MintAccessToken = testMintAccessTokenWithError

		Convey("MintServiceOAuthToken account creation failure retry logic works", func() {

			ctx := testingContext("service@example.com")
			req := &minter.MintServiceOAuthTokenRequest{
				LuciProject: "service-project",
				OauthScope:  []string{"https://www.googleapis.com/auth/cloud-platform"},
			}
			resp, err := rpc.MintServiceOAuthToken(ctx, req)
			So(err, ShouldNotBeNil)
			So(transient.Tag.In(err), ShouldBeTrue)
			So(resp, ShouldBeNil)

			rpc.MintAccessToken = testMintAccessToken
			req = &minter.MintServiceOAuthTokenRequest{
				LuciProject: "service-project",
				OauthScope:  []string{"https://www.googleapis.com/auth/cloud-platform"},
			}
			resp, err = rpc.MintServiceOAuthToken(ctx, req)
			So(err, ShouldBeNil)
			So(resp, ShouldNotBeNil)

		})

	})
}
