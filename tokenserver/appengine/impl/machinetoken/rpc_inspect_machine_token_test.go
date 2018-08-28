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
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	tokenserver "go.chromium.org/luci/tokenserver/api"
	admin "go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/certconfig"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInspectMachineTokenRPC(t *testing.T) {
	Convey("with mocked context", t, func() {
		ctx := testingContext(testingCA)
		signer := testingSigner()
		impl := InspectMachineTokenRPC{Signer: signer}
		tok := testingMachineToken(ctx, signer)

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
				SigningKeyId: signer.KeyNameForTest(),
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
				InvalidityReason: "bad signature - crypto/rsa: verification error",
				Signed:           false,
				NonExpired:       true,
				NonRevoked:       true,
				CertCaName:       "Fake CA: fake.ca",
				SigningKeyId:     signer.KeyNameForTest(),
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
				SigningKeyId:     signer.KeyNameForTest(),
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
				SigningKeyId:     signer.KeyNameForTest(),
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
