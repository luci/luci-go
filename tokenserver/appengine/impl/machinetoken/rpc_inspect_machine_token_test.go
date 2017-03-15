// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package machinetoken

import (
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"

	tokenserver "github.com/luci/luci-go/tokenserver/api"
	admin "github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/appengine/impl/certconfig"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInspectMachineTokenRPC(t *testing.T) {
	Convey("with mocked context", t, func() {
		ctx := testingContext()
		impl := InspectMachineTokenRPC{
			Signer: testingSigner(),
		}
		tok := expectedLuciMachineToken

		Convey("Good token", func() {
			reply, err := impl.InspectMachineToken(ctx, &admin.InspectMachineTokenRequest{
				TokenType: tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN,
				Token:     tok,
			})
			So(err, ShouldBeNil)
			So(reply, ShouldResemble, &admin.InspectMachineTokenResponse{
				Valid:        true,
				Signed:       true,
				NonExpired:   true,
				NonRevoked:   true,
				SigningKeyId: "f9da5a0d0903bda58c6d664e3852a89c283d7fe9",
				CertCaName:   "Fake CA: fake.ca",
				TokenType: &admin.InspectMachineTokenResponse_LuciMachineToken{
					LuciMachineToken: &tokenserver.MachineTokenBody{
						MachineFqdn: "luci-token-server-test-1.fake.domain",
						IssuedBy:    "signer@testing.host",
						IssuedAt:    1422936306,
						Lifetime:    3600,
						CaId:        123,
						CertSn:      4096,
					},
				},
			})
		})

		Convey("Broken signature", func() {
			reply, err := impl.InspectMachineToken(ctx, &admin.InspectMachineTokenRequest{
				TokenType: tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN,
				Token:     tok[:len(tok)-11] + "0" + tok[len(tok)-10:],
			})
			So(err, ShouldBeNil)
			So(reply, ShouldResemble, &admin.InspectMachineTokenResponse{
				Valid:            false,
				InvalidityReason: "can't validate signature - crypto/rsa: verification error",
				Signed:           false,
				NonExpired:       false,
				NonRevoked:       false,
				SigningKeyId:     "f9da5a0d0903bda58c6d664e3852a89c283d7fe9",
				TokenType: &admin.InspectMachineTokenResponse_LuciMachineToken{
					LuciMachineToken: &tokenserver.MachineTokenBody{
						MachineFqdn: "luci-token-server-test-1.fake.domain",
						IssuedBy:    "signer@testing.host",
						IssuedAt:    1422936306,
						Lifetime:    3600,
						CaId:        123,
						CertSn:      4096,
					},
				},
			})
		})

		Convey("Expired", func() {
			clock.Get(ctx).(testclock.TestClock).Add(time.Hour + 11*time.Minute)
			reply, err := impl.InspectMachineToken(ctx, &admin.InspectMachineTokenRequest{
				TokenType: tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN,
				Token:     tok,
			})
			So(err, ShouldBeNil)
			So(reply, ShouldResemble, &admin.InspectMachineTokenResponse{
				Valid:            false,
				InvalidityReason: "expired",
				Signed:           true,
				NonExpired:       false,
				NonRevoked:       true,
				SigningKeyId:     "f9da5a0d0903bda58c6d664e3852a89c283d7fe9",
				CertCaName:       "Fake CA: fake.ca",
				TokenType: &admin.InspectMachineTokenResponse_LuciMachineToken{
					LuciMachineToken: &tokenserver.MachineTokenBody{
						MachineFqdn: "luci-token-server-test-1.fake.domain",
						IssuedBy:    "signer@testing.host",
						IssuedAt:    1422936306,
						Lifetime:    3600,
						CaId:        123,
						CertSn:      4096,
					},
				},
			})
		})

		Convey("Revoked cert", func() {
			// "Revoke" the certificate.
			certconfig.UpdateCRLSet(ctx, "Fake CA: fake.ca", certconfig.CRLShardCount,
				&pkix.CertificateList{
					TBSCertList: pkix.TBSCertificateList{
						RevokedCertificates: []pkix.RevokedCertificate{
							{SerialNumber: big.NewInt(4096)},
						},
					},
				})
			// This makes the token expired too.
			clock.Get(ctx).(testclock.TestClock).Add(time.Hour + 11*time.Minute)
			reply, err := impl.InspectMachineToken(ctx, &admin.InspectMachineTokenRequest{
				TokenType: tokenserver.MachineTokenType_LUCI_MACHINE_TOKEN,
				Token:     tok,
			})
			So(err, ShouldBeNil)
			So(reply, ShouldResemble, &admin.InspectMachineTokenResponse{
				Valid:            false,
				InvalidityReason: "expired", // "expired" 'beats' revocation
				Signed:           true,
				NonExpired:       false,
				NonRevoked:       false, // revoked now!
				SigningKeyId:     "f9da5a0d0903bda58c6d664e3852a89c283d7fe9",
				CertCaName:       "Fake CA: fake.ca",
				TokenType: &admin.InspectMachineTokenResponse_LuciMachineToken{
					LuciMachineToken: &tokenserver.MachineTokenBody{
						MachineFqdn: "luci-token-server-test-1.fake.domain",
						IssuedBy:    "signer@testing.host",
						IssuedAt:    1422936306,
						Lifetime:    3600,
						CaId:        123,
						CertSn:      4096,
					},
				},
			})
		})
	})
}
