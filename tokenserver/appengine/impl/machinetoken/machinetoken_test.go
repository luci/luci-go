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
	"crypto/x509/pkix"
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

func TestMachineFQDN(t *testing.T) {
	// See rpc_mocks_test.go for getTestCert, certWithCN and certWithSAN.

	// Test parsing of the real certs.

	Convey("MachineFQDN works for cert without SAN", t, func() {
		params := MintParams{Cert: getTestCert(certWithCN)}
		fqdn, err := params.MachineFQDN()
		So(err, ShouldBeNil)
		So(fqdn, ShouldEqual, "luci-token-server-test-1.fake.domain")
	})

	Convey("MachineFQDN works for cert with SAN", t, func() {
		params := MintParams{Cert: getTestCert(certWithSAN)}
		fqdn, err := params.MachineFQDN()
		So(err, ShouldBeNil)
		So(fqdn, ShouldEqual, "fuchsia-debian-dev-141242e1-us-central1-f-0psd.c.fuchsia-infra.internal")
	})

	// Test some synthetic cases.

	Convey("MachineFQDN with empty CN", t, func() {
		params := MintParams{
			Cert: &x509.Certificate{
				DNSNames: []string{"name1.example.com"},
			},
		}
		_, err := params.MachineFQDN()
		So(err, ShouldErrLike, "unsupported cert, Subject CN field is required")
	})

	Convey("MachineFQDN with more than one SAN", t, func() {
		params := MintParams{
			Cert: &x509.Certificate{
				Subject:  pkix.Name{CommonName: "name1"},
				DNSNames: []string{"name1.example.com", "name2.example.com"},
			},
		}
		_, err := params.MachineFQDN()
		So(err, ShouldErrLike, "unsupported cert, more than one SAN DNS field")
	})

	Convey("MachineFQDN with SAN and CN not matching", t, func() {
		params := MintParams{
			Cert: &x509.Certificate{
				Subject:  pkix.Name{CommonName: "name1"},
				DNSNames: []string{"name2.example.com"},
			},
		}
		_, err := params.MachineFQDN()
		So(err, ShouldErrLike,
			`unsupported cert, the CN ("name1") should match hostname portion of the SAN DNS ("name2.example.com")`)
	})
}

func TestMintParamsValidation(t *testing.T) {
	Convey("with token params", t, func() {
		params := MintParams{
			Cert: &x509.Certificate{
				Subject:      pkix.Name{CommonName: "host.domain"},
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
			fqdn, err := params.MachineFQDN()
			So(err, ShouldBeNil)
			So(fqdn, ShouldEqual, "host.domain")
		})

		Convey("good params with subdomain", func() {
			params.Cert.Subject.CommonName = "host.subdomain.domain"
			So(params.Validate(), ShouldBeNil)
			fqdn, err := params.MachineFQDN()
			So(err, ShouldBeNil)
			So(fqdn, ShouldEqual, "host.subdomain.domain")
		})

		Convey("bad FQDN case is converted to lowercase", func() {
			params.Cert.Subject.CommonName = "HOST.domain"
			So(params.Validate(), ShouldBeNil)
			fqdn, err := params.MachineFQDN()
			So(err, ShouldBeNil)
			So(fqdn, ShouldEqual, "host.domain")
		})

		Convey("bad FQDN", func() {
			params.Cert.Subject.CommonName = "host"
			So(params.Validate(), ShouldErrLike, "not a valid FQDN")
		})

		Convey("not whitelisted", func() {
			params.Cert.Subject.CommonName = "host.blah"
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
				Cert: &x509.Certificate{
					Subject:      pkix.Name{CommonName: "host.domain"},
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
			So(body, ShouldResembleProto, &tokenserver.MachineTokenBody{
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
