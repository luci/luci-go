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
	"math/big"
	"testing"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/auth/signing"

	"go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/admin/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMintParamsValidation(t *testing.T) {
	Convey("with token params", t, func() {
		params := MintParams{
			FQDN: "host.domain",
			Cert: &x509.Certificate{
				SerialNumber: big.NewInt(12345),
			},
			Config: &admin.CertificateAuthorityConfig{
				KnownDomains: []*admin.DomainConfig{
					{
						Domain:               []string{"domain"},
						MachineTokenLifetime: 3600,
					},
				},
			},
		}

		Convey("good params", func() {
			So(params.Validate(), ShouldBeNil)
		})

		Convey("good params with subdomain", func() {
			params.FQDN = "host.subdomain.domain"
			So(params.Validate(), ShouldBeNil)
		})

		Convey("bad FQDN case", func() {
			params.FQDN = "HOST.domain"
			So(params.Validate(), ShouldErrLike, "expecting FQDN in lowercase")
		})

		Convey("bad FQDN", func() {
			params.FQDN = "host"
			So(params.Validate(), ShouldErrLike, "not a valid FQDN")
		})

		Convey("not whitelisted", func() {
			params.FQDN = "host.blah"
			So(params.Validate(), ShouldErrLike, "not whitelisted in the config")
		})

		Convey("tokens are not allowed", func() {
			params.Config.KnownDomains[0].MachineTokenLifetime = 0
			So(params.Validate(), ShouldErrLike, "are not allowed")
		})

		Convey("bad SN", func() {
			params.Cert.SerialNumber = big.NewInt(-1)
			So(params.Validate(), ShouldErrLike, "invalid certificate serial number")
		})
	})
}

func TestMint(t *testing.T) {
	Convey("with mock context", t, func() {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, time.Date(2015, time.February, 3, 4, 5, 6, 7, time.UTC))

		Convey("works", func() {
			params := MintParams{
				FQDN: "host.domain",
				Cert: &x509.Certificate{
					SerialNumber: big.NewInt(12345),
				},
				Config: &admin.CertificateAuthorityConfig{
					KnownDomains: []*admin.DomainConfig{
						{
							Domain:               []string{"domain"},
							MachineTokenLifetime: 3600,
						},
					},
				},
				Signer: fakeSigner{},
			}
			body, token, err := Mint(ctx, &params)
			So(err, ShouldBeNil)
			So(body, ShouldResemble, &tokenserver.MachineTokenBody{
				MachineFqdn: "host.domain",
				IssuedBy:    "token-server@example.com",
				IssuedAt:    1422936306,
				Lifetime:    3600,
				CaId:        0,
				CertSn:      12345,
			})
			So(token, ShouldEqual, "CjMKC2hvc3QuZG9tYWluEhh0b2tlbi1zZXJ2ZXJAZXhhbXB"+
				"sZS5jb20Y8pHBpgUgkBwwuWASBmtleV9pZBoJc2lnbmF0dXJl")
		})
	})
}

type fakeSigner struct{}

func (fakeSigner) SignBytes(c context.Context, blob []byte) (keyID string, sig []byte, err error) {
	return "key_id", []byte("signature"), nil
}

func (fakeSigner) Certificates(c context.Context) (*signing.PublicCertificates, error) {
	panic("not implemented yet")
}

func (fakeSigner) ServiceInfo(c context.Context) (*signing.ServiceInfo, error) {
	return &signing.ServiceInfo{
		ServiceAccountName: "token-server@example.com",
	}, nil
}
