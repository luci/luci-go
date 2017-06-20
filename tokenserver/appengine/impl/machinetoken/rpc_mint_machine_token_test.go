// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package machinetoken

import (
	"crypto/x509"
	"net"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"

	"github.com/luci/luci-go/tokenserver/api"
	"github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/api/minter/v1"
	"github.com/luci/luci-go/tokenserver/appengine/impl/certconfig"

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
