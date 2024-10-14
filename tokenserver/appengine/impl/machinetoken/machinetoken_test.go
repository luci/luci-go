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
	"context"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth/signing"

	tokenserver "go.chromium.org/luci/tokenserver/api"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
)

func TestMachineFQDN(t *testing.T) {
	// See rpc_mocks_test.go for getTestCert, certWithCN and certWithSAN.

	// Test parsing of the real certs.

	ftt.Run("MachineFQDN works for cert without SAN", t, func(t *ftt.Test) {
		params := MintParams{Cert: getTestCert(certWithCN)}
		fqdn, err := params.MachineFQDN()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fqdn, should.Equal("luci-token-server-test-1.fake.domain"))
	})

	ftt.Run("MachineFQDN works for cert with SAN", t, func(t *ftt.Test) {
		params := MintParams{Cert: getTestCert(certWithSAN)}
		fqdn, err := params.MachineFQDN()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fqdn, should.Equal("fuchsia-debian-dev-141242e1-us-central1-f-0psd.c.fuchsia-infra.internal"))
	})

	ftt.Run("MachineFQDN works for cert where CN == SAN", t, func(t *ftt.Test) {
		params := MintParams{Cert: getTestCert(certWithCNEqualSAN)}
		fqdn, err := params.MachineFQDN()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fqdn, should.Equal("proto-chrome-focal.c.chromecompute.google.com.internal"))
	})

	ftt.Run("MachineFQDN with more than one SAN", t, func(t *ftt.Test) {
		params := MintParams{
			Cert: &x509.Certificate{
				Subject:  pkix.Name{CommonName: "name1"},
				DNSNames: []string{"name1.example.com", "name2.example.com"},
			},
		}
		fqdn, err := params.MachineFQDN()
		assert.Loosely(t, fqdn, should.Equal("name1.example.com"))
		assert.Loosely(t, err, should.BeNil)
	})

	// Test some synthetic cases.

	ftt.Run("MachineFQDN with empty CN", t, func(t *ftt.Test) {
		params := MintParams{
			Cert: &x509.Certificate{
				DNSNames: []string{"name1.example.com"},
			},
		}
		_, err := params.MachineFQDN()
		assert.Loosely(t, err, should.ErrLike("unsupported cert, Subject CN field is required"))
	})
}

func TestMintParamsValidation(t *testing.T) {
	ftt.Run("with token params", t, func(t *ftt.Test) {
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

		t.Run("good params", func(t *ftt.Test) {
			assert.Loosely(t, params.Validate(), should.BeNil)
			fqdn, err := params.MachineFQDN()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fqdn, should.Equal("host.domain"))
		})

		t.Run("good params with subdomain", func(t *ftt.Test) {
			params.Cert.Subject.CommonName = "host.subdomain.domain"
			assert.Loosely(t, params.Validate(), should.BeNil)
			fqdn, err := params.MachineFQDN()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fqdn, should.Equal("host.subdomain.domain"))
		})

		t.Run("bad FQDN case is converted to lowercase", func(t *ftt.Test) {
			params.Cert.Subject.CommonName = "HOST.domain"
			assert.Loosely(t, params.Validate(), should.BeNil)
			fqdn, err := params.MachineFQDN()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fqdn, should.Equal("host.domain"))
		})

		t.Run("bad FQDN", func(t *ftt.Test) {
			params.Cert.Subject.CommonName = "host"
			assert.Loosely(t, params.Validate(), should.ErrLike("not a valid FQDN"))
		})

		t.Run("not listed", func(t *ftt.Test) {
			params.Cert.Subject.CommonName = "host.blah"
			assert.Loosely(t, params.Validate(), should.ErrLike("not listed in the config"))
		})

		t.Run("tokens are not allowed", func(t *ftt.Test) {
			params.Config.KnownDomains[0].MachineTokenLifetime = 0
			assert.Loosely(t, params.Validate(), should.ErrLike("are not allowed"))
		})

		t.Run("bad SN", func(t *ftt.Test) {
			params.Cert.SerialNumber = big.NewInt(-1)
			assert.Loosely(t, params.Validate(), should.ErrLike("invalid certificate serial number"))
		})
	})
}

func TestMint(t *testing.T) {
	ftt.Run("with mock context", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, time.Date(2015, time.February, 3, 4, 5, 6, 7, time.UTC))

		t.Run("works", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, body, should.Resemble(&tokenserver.MachineTokenBody{
				MachineFqdn: "host.domain",
				IssuedBy:    "token-server@example.com",
				IssuedAt:    1422936306,
				Lifetime:    3600,
				CaId:        0,
				CertSn:      big.NewInt(12345).Bytes(),
			}))
			assert.Loosely(t, token, should.Equal("CjQKC2hvc3QuZG9tYWluEhh0b2tlbi1zZXJ2ZXJAZXhhbXB"+
				"sZS5jb20Y8pHBpgUgkBw6AjA5EgZrZXlfaWQaCXNpZ25hdHVyZQ"))
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
