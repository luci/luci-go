// Copyright 2016 The LUCI Authors.
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

package machinetoken

import (
	"crypto/x509"
	"net"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/certconfig"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMintMachineTokenRPC(t *testing.T) {
	t.Parallel()

	Convey("Successful RPC", t, func() {
		ctx := auth.WithState(testingContext(testingCA), &authtest.FakeState{
			PeerIPOverride: net.ParseIP("127.10.10.10"),
		})

		var loggedInfo *MintedTokenInfo
		impl := MintMachineTokenRPC{
			Signer: testingSigner(),
			CheckCertificate: func(_ context.Context, cert *x509.Certificate) (*certconfig.CA, error) {
				return &testingCA, nil
			},
			LogToken: func(c context.Context, info *MintedTokenInfo) error {
				loggedInfo = info
				return nil
			},
		}

		resp, err := impl.MintMachineToken(ctx, testingMachineTokenRequest(ctx))
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &minter.MintMachineTokenResponse{
			ServiceVersion: "unit-tests/mocked-ver",
			TokenResponse: &minter.MachineTokenResponse{
				ServiceVersion: "unit-tests/mocked-ver",
				TokenType: &minter.MachineTokenResponse_LuciMachineToken{
					LuciMachineToken: &minter.LuciMachineToken{
						MachineToken: expectedLuciMachineToken,
						Expiry:       google.NewTimestamp(clock.Now(ctx).Add(time.Hour)),
					},
				},
			},
		})

		So(loggedInfo, ShouldResemble, &MintedTokenInfo{
			Request:  testingRawRequest(ctx),
			Response: resp.TokenResponse,
			TokenBody: &tokenserver.MachineTokenBody{
				MachineFqdn: "luci-token-server-test-1.fake.domain",
				IssuedBy:    "signer@testing.host",
				IssuedAt:    1422936306,
				Lifetime:    3600,
				CaId:        123,
				CertSn:      4096,
			},
			CA:        &testingCA,
			PeerIP:    net.ParseIP("127.10.10.10"),
			RequestID: "gae-request-id",
		})
	})

	Convey("Unsuccessful RPC", t, func() {
		// Modify testing CA to have no domains whitelisted.
		testingCA := certconfig.CA{
			CN: "Fake CA: fake.ca",
			ParsedConfig: &admin.CertificateAuthorityConfig{
				UniqueId: 123,
			},
		}
		ctx := auth.WithState(testingContext(testingCA), &authtest.FakeState{
			PeerIPOverride: net.ParseIP("127.10.10.10"),
		})

		impl := MintMachineTokenRPC{
			Signer: testingSigner(),
			CheckCertificate: func(_ context.Context, cert *x509.Certificate) (*certconfig.CA, error) {
				return &testingCA, nil
			},
			LogToken: func(c context.Context, info *MintedTokenInfo) error {
				panic("must not be called") // we log only successfully generated tokens
			},
		}

		// This request is structurally valid, but forbidden by CA config. It
		// generates MintMachineTokenResponse with non-zero error code.
		resp, err := impl.MintMachineToken(ctx, testingMachineTokenRequest(ctx))
		So(err, ShouldBeNil)
		So(resp, ShouldResemble, &minter.MintMachineTokenResponse{
			ServiceVersion: "unit-tests/mocked-ver",
			ErrorCode:      minter.ErrorCode_BAD_TOKEN_ARGUMENTS,
			ErrorMessage:   `the domain "fake.domain" is not whitelisted in the config`,
		})
	})
}
