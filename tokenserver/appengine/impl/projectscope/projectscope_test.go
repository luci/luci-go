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
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"go.chromium.org/luci/common/proto/google"

	"go.chromium.org/luci/common/gcloud/iam"

	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectscope"
	"golang.org/x/oauth2"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth/authdb"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/appengine/gaetesting"

	"go.chromium.org/luci/server/auth/signing"

	"go.chromium.org/luci/tokenserver/api/admin/v1"

	"go.chromium.org/luci/tokenserver/appengine/impl/certconfig"

	"go.chromium.org/luci/tokenserver/appengine/impl/delegation"
	"go.chromium.org/luci/tokenserver/appengine/impl/machinetoken"
	"go.chromium.org/luci/tokenserver/appengine/impl/serviceaccounts"

	"go.chromium.org/luci/tokenserver/api/minter/v1"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeConfigItem struct {
	ServiceName string
	GcpProject  string
	LuciProject string
}

const (
	scopedServiceName        = "user:service@example.com"
	scopedServiceGcpProject  = "scoped-service-project-1234"
	scopedServiceLuciProject = "testing-luci-project"

	doubleCreateServiceName        = "user:doublecreate@example.com"
	doubleCreateServiceGcpProject  = "double-created-project-1234"
	doubleCreateServiceLuciProject = "some-luci-project"

	forLookupServiceName        = scopedServiceName
	forLookupServiceLuciProject = "lookup-luci-project"

	nonPreExistingLuciProject = "non-pre-existing-luci-project"

	testingRevision = "test-revision"
	testingToken    = "testing-token"
)

var (
	testingOauthScopes = []string{"https://www.googleapis.com/scope1"}

	scopedServiceFakeConfig = &fakeConfigItem{
		ServiceName: scopedServiceName,
		GcpProject:  scopedServiceGcpProject,
		LuciProject: scopedServiceLuciProject,
	}

	doubleCreateFakeConfig = &fakeConfigItem{
		ServiceName: doubleCreateServiceName,
		GcpProject:  doubleCreateServiceGcpProject,
		LuciProject: doubleCreateServiceLuciProject,
	}

	forLookupFakeConfig = &fakeConfigItem{
		ServiceName: forLookupServiceName,
		GcpProject:  scopedServiceGcpProject,
		LuciProject: forLookupServiceLuciProject,
	}
)

type minterServerImpl struct {
	machinetoken.MintMachineTokenRPC
	delegation.MintDelegationTokenRPC
	serviceaccounts.MintOAuthTokenGrantRPC
	serviceaccounts.MintOAuthTokenViaGrantRPC
	MintServiceOAuthTokenRPC
}

type adminServerImpl struct {
	certconfig.ImportCAConfigsRPC
	delegation.ImportDelegationConfigsRPC
	delegation.InspectDelegationTokenRPC
	machinetoken.InspectMachineTokenRPC
	serviceaccounts.ImportServiceAccountsConfigsRPC
	serviceaccounts.InspectOAuthTokenGrantRPC
	CreateProjectScopedServiceAccountRPC
	LookupProjectScopedServiceAccountRPC
	ImportProjectScopedServiceAccountsConfigsRPC
}

type testingSigner struct {
}

func (s *testingSigner) SignBytes(c context.Context, blob []byte) (keyName string, signature []byte, err error) {
	panic("testingSigner.SignBytes: Not implemented")
	return "", []byte{}, nil
}

func (s *testingSigner) Certificates(c context.Context) (*signing.PublicCertificates, error) {
	panic("testingSigner.Certificates: Not implemented")
	return nil, nil
}

func (s *testingSigner) ServiceInfo(c context.Context) (*signing.ServiceInfo, error) {
	return &signing.ServiceInfo{
		AppID:              "testing-id",
		AppRuntime:         "testing",
		AppRuntimeVersion:  "testing",
		AppVersion:         "testing",
		ServiceAccountName: "local-unit-test",
	}, nil
}

func testingContext(caller identity.Identity) context.Context {
	ctx := gaetesting.TestingContext()
	ctx = logging.SetLevel(ctx, logging.Debug)
	ctx = info.GetTestable(ctx).SetRequestID("gae-request-id")
	ctx, _ = testclock.UseTime(ctx, testclock.TestTimeUTC)
	return auth.WithState(ctx, &authtest.FakeState{
		Identity:       caller,
		PeerIPOverride: net.ParseIP("127.10.10.10"),
		FakeDB:         &authdb.SnapshotDB{Rev: 1234},
	})
}

func newAdminServer(tc testclock.TestClock, identities projectscope.ScopedIdentityManager, rules func(ctx context.Context) (*Rules, error)) admin.AdminServer {
	return &adminServerImpl{
		CreateProjectScopedServiceAccountRPC: CreateProjectScopedServiceAccountRPC{
			Rules:            rules,
			ScopedIdentities: identities,
		},
		LookupProjectScopedServiceAccountRPC: LookupProjectScopedServiceAccountRPC{
			ScopedIdentities: identities,
		},
		ImportProjectScopedServiceAccountsConfigsRPC: ImportProjectScopedServiceAccountsConfigsRPC{
			RulesCache: nil,
		},
	}
}

func newMinterServer(tc testclock.TestClock, signer signing.Signer, rules func(ctx context.Context) (*Rules, error), identities projectscope.ScopedIdentityManager) minter.TokenMinterServer {
	serviceAccountMap := map[string]*iam.ServiceAccount{
		projectscope.GenerateAccountId(scopedServiceName, scopedServiceLuciProject): {
			Id:             "1234567",
			Name:           "",
			ProjectId:      scopedServiceGcpProject,
			UniqueId:       "1234567",
			Email:          "foo@bar.com",
			DisplayName:    "",
			Oauth2ClientId: "",
		},
		projectscope.GenerateAccountId(scopedServiceName, nonPreExistingLuciProject): {
			Id:             "1234568",
			Name:           "",
			ProjectId:      scopedServiceGcpProject,
			UniqueId:       "1234568",
			Email:          "barfoo@bar.com",
			DisplayName:    "",
			Oauth2ClientId: "",
		},
	}

	mint := func(ctx context.Context, params auth.MintAccessTokenParams) (*oauth2.Token, error) {
		return &oauth2.Token{
			AccessToken:  testingToken,
			TokenType:    "Bearer",
			RefreshToken: "",
			Expiry:       tc.Now().Add(time.Second * 200),
		}, nil
	}
	log := func(ctx context.Context, tokenInfo serviceaccounts.LoggableOAuthTokenInfo) error {
		return nil
	}
	createSa := func(ctx context.Context, gcpProject string, identity *projectscope.ScopedIdentity) (*iam.ServiceAccount, bool, error) {
		if serviceAccount, found := serviceAccountMap[identity.AccountId]; found {
			return serviceAccount, false, nil
		}

		_ = "ServiceAccountCreator - gcpProject: scoped-service-project-1234, identity: &{ ps-3237e59549d7f3351d49db562a0 user:service@example.com non-pre-existing-luci-project scoped-service-project-1234 false { }}"

		// Expecting service accounts to be previously known in tests
		panic(fmt.Sprintf("ServiceAccountCreator - gcpProject: %v, identity: %v", gcpProject, identity))
	}

	return &minterServerImpl{
		MintServiceOAuthTokenRPC: MintServiceOAuthTokenRPC{
			Signer:               signer,
			Rules:                rules,
			MintAccessToken:      mint,
			LogOAuthToken:        log,
			Storage:              identities,
			CreateServiceAccount: createSa,
		},
	}
}

func newServers(tc testclock.TestClock) (admin.AdminServer, minter.TokenMinterServer) {
	signer := &testingSigner{}
	identities := projectscope.ScopedIdentities
	rules := func(ctx context.Context) (*Rules, error) {
		return &Rules{
			revision: testingRevision,
			rulesPerService: map[string]*Rule{
				scopedServiceName: {
					CloudProjectID: scopedServiceGcpProject,
					Rule: &admin.ServiceConfig{
						Service:        scopedServiceName,
						CloudProjectId: scopedServiceGcpProject,
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
			},
		}, nil
	}
	return newAdminServer(tc, identities, rules), newMinterServer(tc, signer, rules, identities)
}

/*
Test scenarios:

Good cases
  1.1 SA created explicitly, then minted
  1.2 SA created implicitly, then minted
  1.3 Existing SA looked up

Error cases
  2.1 Create SA twice
  2.2 Create SA without config
  2.3 Lookup non existing SA
  2.4 Mint for non existing SA
*/
func TestProjectScopedServiceAccounts(t *testing.T) {
	t.Parallel()

	Convey("Happy path / positive cases", t, func() {

		minterCtx := testingContext(scopedServiceName)
		adminCtx := gaetesting.TestingContext()
		adminCtx, tc := testclock.UseTime(adminCtx, testclock.TestTimeUTC)
		minterCtx, _ = testclock.UseTime(minterCtx, testclock.TestTimeUTC)
		adminServer, minterServer := newServers(tc)
		So(tc.Now(), ShouldResemble, tc.Now())

		Convey("SA created explicitly, then token minting", func() {
			createSaResp, err := adminServer.CreateProjectScopedServiceAccount(adminCtx, &admin.CreateProjectScopedServiceAccountRequest{
				Service: scopedServiceName,
				Project: scopedServiceLuciProject,
			})
			So(err, ShouldBeNil)
			So(createSaResp, ShouldResemble, &admin.CreateProjectScopedServiceAccountResponse{
				AccountId: projectscope.GenerateAccountId(scopedServiceName, scopedServiceLuciProject),
			})

			mintResp, err := minterServer.MintServiceOAuthToken(minterCtx, &minter.MintServiceOAuthTokenRequest{
				LuciProject:         scopedServiceLuciProject,
				OauthScope:          []string{"https://www.googleapis.com/scope1"},
				MinValidityDuration: 300,
			})
			So(err, ShouldBeNil)
			So(mintResp, ShouldResemble, &minter.MintServiceOAuthTokenResponse{
				ServiceAccount: "foo@bar.com",
				AccessToken:    testingToken,
				Expiry:         google.NewTimestamp(tc.Now().Add(time.Second * 200)),
				ServiceVersion: "testing-id/testing",
			})

			Convey("Existing SA looked up", func() {
				lookupSaResp, err := adminServer.LookupProjectScopedServiceAccount(adminCtx, &admin.LookupProjectScopedServiceAccountRequest{
					AccountId: projectscope.GenerateAccountId(scopedServiceName, scopedServiceLuciProject),
				})
				So(err, ShouldBeNil)
				So(lookupSaResp, ShouldResemble, &admin.LookupProjectScopedServiceAccountResponse{
					Service: scopedServiceName,
					Project: scopedServiceLuciProject,
				})
			})

		})

		Convey("SA created implicitly by minting", func() {
			mintResp, err := minterServer.MintServiceOAuthToken(minterCtx, &minter.MintServiceOAuthTokenRequest{
				LuciProject:         nonPreExistingLuciProject,
				OauthScope:          testingOauthScopes,
				MinValidityDuration: 200,
			})
			So(err, ShouldBeNil)
			So(mintResp, ShouldResemble, &minter.MintServiceOAuthTokenResponse{
				ServiceAccount: "barfoo@bar.com",
				AccessToken:    testingToken,
				Expiry:         google.NewTimestamp(tc.Now().Add(time.Second * 200)),
				ServiceVersion: "testing-id/testing",
			})
		})

		Convey("Fail to lookup, create account, succeed to lookup", func() {
			_, err := adminServer.LookupProjectScopedServiceAccount(adminCtx, &admin.LookupProjectScopedServiceAccountRequest{
				AccountId: projectscope.GenerateAccountId(forLookupServiceName, forLookupServiceLuciProject),
			})
			So(err, ShouldNotBeNil)

			createSaResp, err := adminServer.CreateProjectScopedServiceAccount(adminCtx, &admin.CreateProjectScopedServiceAccountRequest{
				Service: forLookupServiceName,
				Project: forLookupServiceLuciProject,
			})
			So(err, ShouldBeNil)
			So(createSaResp, ShouldResemble, &admin.CreateProjectScopedServiceAccountResponse{
				AccountId: projectscope.GenerateAccountId(forLookupServiceName, forLookupServiceLuciProject),
			})

			lookupResp, err := adminServer.LookupProjectScopedServiceAccount(adminCtx, &admin.LookupProjectScopedServiceAccountRequest{
				AccountId: projectscope.GenerateAccountId(forLookupServiceName, forLookupServiceLuciProject),
			})
			So(err, ShouldBeNil)
			So(lookupResp, ShouldResemble, &admin.LookupProjectScopedServiceAccountResponse{
				Service: forLookupServiceName,
				Project: forLookupServiceLuciProject,
			})
		})

		Convey("Mint with error, change config, then mint successfully", func() {
			/*
				_, err := minterServer.MintServiceOAuthToken(minterCtx, &minter.MintServiceOAuthTokenRequest{})
				So(err, ShouldNotBeNil)

				config, err := adminServer.ImportProjectScopedServiceAccountsConfigs(adminCtx, nil)
				So(err, ShouldBeNil)
				So(config, ShouldResemble, nil)

				mintResp, err := minterServer.MintServiceOAuthToken(minterCtx, &minter.MintServiceOAuthTokenRequest{})
				So(err, ShouldBeNil)
				So(mintResp, ShouldResemble, &minter.MintServiceOAuthTokenResponse{})
			*/
		})

	})

	Convey("Error cases / negative cases", t, func() {

		minterCtx := testingContext("user:user@example.com")
		adminCtx := gaetesting.TestingContext()
		adminCtx, tc := testclock.UseTime(adminCtx, testclock.TestTimeUTC)
		minterCtx, _ = testclock.UseTime(minterCtx, testclock.TestTimeUTC)
		adminServer, minterServer := newServers(tc)

		Convey("Create SA twice", func() {
			createSaResp, err := adminServer.CreateProjectScopedServiceAccount(adminCtx, &admin.CreateProjectScopedServiceAccountRequest{
				Service: doubleCreateServiceName,
				Project: doubleCreateServiceLuciProject,
			})
			So(err, ShouldBeNil)
			So(createSaResp, ShouldResemble, &admin.CreateProjectScopedServiceAccountResponse{
				AccountId: projectscope.GenerateAccountId(doubleCreateServiceName, doubleCreateServiceLuciProject),
			})

			createSaResp, err = adminServer.CreateProjectScopedServiceAccount(adminCtx, &admin.CreateProjectScopedServiceAccountRequest{
				Service: doubleCreateServiceName,
				Project: doubleCreateServiceLuciProject,
			})
			So(err, ShouldNotBeNil)
		})

		Convey("Create SA without config", func() {
			_, err := adminServer.CreateProjectScopedServiceAccount(adminCtx, &admin.CreateProjectScopedServiceAccountRequest{})
			So(err, ShouldNotBeNil)
		})

		Convey("Lookup non-existing SA", func() {
			_, err := adminServer.LookupProjectScopedServiceAccount(adminCtx, &admin.LookupProjectScopedServiceAccountRequest{})
			So(err, ShouldNotBeNil)
		})

		Convey("Mint for non-existing SA", func() {
			_, err := minterServer.MintServiceOAuthToken(minterCtx, &minter.MintServiceOAuthTokenRequest{})
			So(err, ShouldNotBeNil)
		})

	})

}
