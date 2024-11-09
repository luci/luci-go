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

package certchecker

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/certconfig"
)

func TestCertChecker(t *testing.T) {
	ftt.Run("CertChecker works", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()
		ctx = cryptorand.MockForTest(ctx, 0)
		ctx, clk := testclock.UseTime(ctx, testclock.TestTimeUTC)

		// Generate new CA private key and certificate.
		pkey, caCert, err := generateCA(ctx, "Some CA: ca-name.fake")
		assert.Loosely(t, err, should.BeNil)

		// Generate phony CA config.
		configBlob, _ := proto.Marshal(&admin.CertificateAuthorityConfig{
			UniqueId: 0,
			Cn:       "Some CA: ca-name.fake",
			CrlUrl:   "http://example.com",
		})

		// Nothing in the datastore yet.
		checker, err := GetCertChecker(ctx, "Some CA: ca-name.fake")
		assert.Loosely(t, err, should.ErrLike("no such CA"))

		// Put it into the datastore.
		caEntity := certconfig.CA{
			CN:     "Some CA: ca-name.fake",
			Config: configBlob,
			Cert:   caCert,
			Ready:  true,
		}
		err = datastore.Put(ctx, &caEntity)
		assert.Loosely(t, err, should.BeNil)

		// In the datastore now.
		checker, err = GetCertChecker(ctx, "Some CA: ca-name.fake")
		assert.Loosely(t, err, should.BeNil)

		// Update associated CRL (it's empty).
		err = certconfig.UpdateCRLSet(ctx, "Some CA: ca-name.fake",
			certconfig.CRLShardCount, &pkix.CertificateList{})
		assert.Loosely(t, err, should.BeNil)

		// Generate some certificate signed by the CA.
		certDer, err := generateCert(ctx, 2, "some-cert-name.fake", caCert, pkey)
		assert.Loosely(t, err, should.BeNil)

		// Use CertChecker to check its validity. Need to parse DER first.
		parsedCert, err := x509.ParseCertificate(certDer)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, parsedCert.Issuer.CommonName, should.Equal("Some CA: ca-name.fake"))

		// Valid!
		ca, err := checker.CheckCertificate(ctx, parsedCert)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ca.CN, should.Equal("Some CA: ca-name.fake"))
		assert.Loosely(t, ca.ParsedConfig, should.NotBeNil)

		// Revoke the certificate by generating new CRL and putting it into the
		// datastore.
		err = certconfig.UpdateCRLSet(ctx, "Some CA: ca-name.fake", certconfig.CRLShardCount,
			&pkix.CertificateList{
				TBSCertList: pkix.TBSCertificateList{
					RevokedCertificates: []pkix.RevokedCertificate{
						{SerialNumber: big.NewInt(2)},
					},
				},
			})
		assert.Loosely(t, err, should.BeNil)

		// Bump time to invalidate cert checker caches.
		clk.Add(10 * time.Minute)

		// Check same cert again. Should be rejected now as revoked.
		_, err = checker.CheckCertificate(ctx, parsedCert)
		assert.Loosely(t, err, should.ErrLike("certificate with SN 2 has been revoked"))

		// Fast forward past cert expiration time.
		clk.Add(6 * time.Hour)

		// Should be rejected as expired now.
		_, err = checker.CheckCertificate(ctx, parsedCert)
		assert.Loosely(t, err, should.ErrLike("certificate has expired"))

		// Generate a cert signed by a CA with the same name, but with different
		// unexpected CA keys.
		phonyCAKey, phonyCACert, err := generateCA(ctx, "Some CA: ca-name.fake")
		assert.Loosely(t, err, should.BeNil)
		certDer, err = generateCert(ctx, 3, "some-name", phonyCACert, phonyCAKey)
		assert.Loosely(t, err, should.BeNil)

		// CertChecker rejects it.
		parsedCert, _ = x509.ParseCertificate(certDer)
		_, err = checker.CheckCertificate(ctx, parsedCert)
		assert.Loosely(t, err, should.ErrLike("crypto/rsa: verification error"))
	})
}

func generateCA(c context.Context, name string) (*rsa.PrivateKey, []byte, error) {
	// See https://golang.org/src/crypto/tls/generate_cert.go.
	rand := cryptorand.Get(c)
	privKey, err := rsa.GenerateKey(rand, 512) // use short key in tests
	if err != nil {
		return nil, nil, err
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             clock.Now(c),
		NotAfter:              clock.Now(c).Add(30 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	derBytes, err := x509.CreateCertificate(rand, &template, &template, privKey.Public(), privKey)
	if err != nil {
		return nil, nil, err
	}
	return privKey, derBytes, nil
}

func generateCert(c context.Context, sn int64, name string, caCert []byte, caKey *rsa.PrivateKey) ([]byte, error) {
	parent, err := x509.ParseCertificate(caCert)
	if err != nil {
		return nil, err
	}
	rand := cryptorand.Get(c)
	privKey, err := rsa.GenerateKey(rand, 512) // use short key in tests
	if err != nil {
		return nil, err
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(sn),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             clock.Now(c),
		NotAfter:              clock.Now(c).Add(5 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	return x509.CreateCertificate(rand, &template, parent, privKey.Public(), caKey)
}
