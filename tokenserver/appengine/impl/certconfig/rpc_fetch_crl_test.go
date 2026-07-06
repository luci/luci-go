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

package certconfig

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
)

func TestFetchCRLRPC(t *testing.T) {
	ftt.Run("with mock context", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()
		ctx, _ = testclock.UseTime(ctx, time.Date(2016, 3, 16, 0, 0, 0, 0, time.UTC))
		ctx = auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
			cfg.AnonymousTransport = func(context.Context) http.RoundTripper {
				return http.DefaultTransport // mock URLFetch service
			}
			return cfg
		})

		importConfig := func(cfg string) {
			impl := ImportCAConfigsRPC{}
			_, err := impl.ImportCAConfigs(prepareCfg(ctx, cfg), nil)
			if err != nil {
				panic(err)
			}
		}

		callFetchCRL := func(cn string, force bool) error {
			impl := FetchCRLRPC{}
			_, err := impl.FetchCRL(ctx, &admin.FetchCRLRequest{
				Cn:    cn,
				Force: force,
			})
			return err
		}

		t.Run("FetchCRL not configured", func(t *ftt.Test) {
			// Prepare config (with empty crl_url).
			importConfig(`
				certificate_authority {
					cn: "Puppet CA: fake.ca"
					cert_path: "certs/fake.ca.crt"
				}
			`)
			// Use it, must succeed without doing anything.
			err := callFetchCRL("Puppet CA: fake.ca", false)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("FetchCRL works (der, no etags)", func(t *ftt.Test) {
			ts := serveCRL()
			defer ts.Close()

			// Prepare config.
			importConfig(fmt.Sprintf(`
				certificate_authority {
					cn: "Puppet CA: fake.ca"
					cert_path: "certs/fake.ca.crt"
					crl_url: %q
				}
			`, ts.URL))

			// Import works.
			ts.CRL = fakeCACrl
			err := callFetchCRL("Puppet CA: fake.ca", true)
			assert.Loosely(t, err, should.BeNil)

			// CRL is there.
			crl := CRL{
				Parent: ds.NewKey(ctx, "CA", "Puppet CA: fake.ca", 0, nil),
			}
			err = ds.Get(ctx, &crl)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, crl.RevokedCertsCount, should.Equal(1)) // fakeCACrl has only 1 SN
		})

		t.Run("FetchCRL works (pem, no etags)", func(t *ftt.Test) {
			ts := serveCRL()
			defer ts.Close()

			// Prepare config.
			importConfig(fmt.Sprintf(`
				certificate_authority {
					cn: "Puppet CA: fake.ca"
					cert_path: "certs/fake.ca.crt"
					crl_url: %q
				}
			`, ts.URL))

			// Import works.
			ts.CRL = fakeCACrl
			ts.ServePEM = true
			err := callFetchCRL("Puppet CA: fake.ca", true)
			assert.Loosely(t, err, should.BeNil)

			// CRL is there.
			crl := CRL{
				Parent: ds.NewKey(ctx, "CA", "Puppet CA: fake.ca", 0, nil),
			}
			err = ds.Get(ctx, &crl)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, crl.RevokedCertsCount, should.Equal(1)) // fakeCACrl has only 1 SN
		})

		t.Run("FetchCRL works (der, with etags)", func(t *ftt.Test) {
			ts := serveCRL()
			defer ts.Close()

			// Prepare config.
			importConfig(fmt.Sprintf(`
				certificate_authority {
					cn: "Puppet CA: fake.ca"
					cert_path: "certs/fake.ca.crt"
					crl_url: %q
				}
			`, ts.URL))

			// Initial import works.
			ts.CRL = fakeCACrl
			ts.Etag = `"etag1"`
			assert.Loosely(t, callFetchCRL("Puppet CA: fake.ca", false), should.BeNil)

			// CRL is there.
			crl := CRL{
				Parent: ds.NewKey(ctx, "CA", "Puppet CA: fake.ca", 0, nil),
			}
			err := ds.Get(ctx, &crl)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, crl.LastFetchETag, should.Equal(`"etag1"`))
			assert.Loosely(t, crl.EntityVersion, should.Equal(1))

			// Refetch. No etag change.
			assert.Loosely(t, callFetchCRL("Puppet CA: fake.ca", false), should.BeNil)

			// Entity isn't touched.
			err = ds.Get(ctx, &crl)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, crl.LastFetchETag, should.Equal(`"etag1"`))
			assert.Loosely(t, crl.EntityVersion, should.Equal(1))

			// Refetch. Etag changes.
			ts.Etag = `"etag2"`
			assert.Loosely(t, callFetchCRL("Puppet CA: fake.ca", false), should.BeNil)

			// Entity is updated.
			err = ds.Get(ctx, &crl)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, crl.LastFetchETag, should.Equal(`"etag2"`))
			assert.Loosely(t, crl.EntityVersion, should.Equal(2))
		})
	})
}

// Valid CRL signed by key that corresponds to fakeCACrt.
//
// Contains only one revoked SN: "2".
const fakeCACrl = `-----BEGIN X509 CRL-----
MIICuzCBpAIBATANBgkqhkiG9w0BAQUFADAdMRswGQYDVQQDDBJQdXBwZXQgQ0E6
IGZha2UuY2EXDTE2MDMxNTAzNDk0NloXDTIxMDMxNDAzNDk0N1owIjAgAgECFw0x
NjAzMTUwMzQ5NDdaMAwwCgYDVR0VBAMKAQGgLzAtMB8GA1UdIwQYMBaAFOeGP1Os
e9spvhIIrGMEZEpoeiDqMAoGA1UdFAQDAgEBMA0GCSqGSIb3DQEBBQUAA4ICAQA8
LeRLqrgl1ed5UbFQyWnmpOW58PzIDEdCtRutVc12VlMKu+FyJ6DELXDpmZjkam32
gMrH9zHbLywO3O6qGl8WaKMVPhKyhdemQa9/TrqFr/lqEsfM9g6ZY4b3dO9VFy42
9SMTQF6iu7ZRfhjui50DZlbD+VtfgTAJpeVTKR3E6ntuYQ+noJ568xcwcswAR6hT
iAvv49kExuflo2ntg9uSHZYvo/PMmUZZ/ThMK+EfalWsz//N1JOSahLl1qakEBKz
OD6QsZB0K3160hsPO5O8iC2FdYa1xiamTiYOKAIqIRgX8+WH2cfc4Wg8mGz4DtJE
BlPZCIhxjbzymi55B2N1Mo/KuYD73j24NN6IG7s6JSohjn/In7h7T9gkOGwkxM5P
jZrNiLYELrfMMVl9z3uiA31qVPoVa2MPsfwY3pWtTVZ3lJ/mWAFesrgCl2FSgBcr
t2WZsEUA7W8l45nbNg8m8l+nOEBCM7Pjycy8ZV7XFdT9iATn44huQi1CGw2xUpEX
8FOcDDS2tb78R3ZoyqFS5l/P5Kd0DitivPhRNQXQboFqT5XL9EBKcyExnR+y72+B
7fIzS92HZavZYpO/YKHweFWonSuNcGOwqLyI/ZZealwOQROD4AC6ZMUeY9oQkbEE
3QbCiGRlaGEOA9SCEoSTNPN9LQ1nHKoaFDy1B5ralA==
-----END X509 CRL-----
`

type crlServer struct {
	*httptest.Server

	Lock     sync.Mutex
	CRL      string
	Etag     string
	ServePEM bool
}

// serveCRL starts a test server that serves CRL file.
func serveCRL() *crlServer {
	s := &crlServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Lock.Lock()
		defer s.Lock.Unlock()

		var blob []byte
		if s.ServePEM {
			blob = []byte(s.CRL)
		} else {
			der, err := utils.ParsePEM(s.CRL, "X509 CRL")
			if err != nil {
				w.WriteHeader(500)
				return
			}
			blob = der
		}

		if s.Etag != "" {
			w.Header().Set("ETag", s.Etag)
		}
		w.WriteHeader(200)
		w.Write(blob)
	}))
	return s
}

func TestCRLRollback(t *testing.T) {
	ftt.Run("CRL Rollback Protection", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()
		ctx, _ = testclock.UseTime(ctx, time.Date(2020, 6, 1, 12, 0, 0, 0, time.UTC))

		// Generate CA
		priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		assert.Loosely(t, err, should.BeNil)

		template := x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject: pkix.Name{
				CommonName: "Test CA",
			},
			NotBefore:             time.Now().Add(-1 * time.Hour),
			NotAfter:              time.Now().Add(24 * time.Hour),
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
		}

		caBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
		assert.Loosely(t, err, should.BeNil)

		caCert, err := x509.ParseCertificate(caBytes)
		assert.Loosely(t, err, should.BeNil)

		caEntity := &CA{
			CN:   "Test CA",
			Cert: caBytes,
		}
		err = ds.Put(ctx, caEntity)
		assert.Loosely(t, err, should.BeNil)

		// Helper to sign CRL
		signCRL := func(thisUpdate, nextUpdate time.Time, revokedSerialNumbers []int64) []byte {
			revoked := make([]pkix.RevokedCertificate, len(revokedSerialNumbers))
			for i, sn := range revokedSerialNumbers {
				revoked[i] = pkix.RevokedCertificate{
					SerialNumber:   big.NewInt(sn),
					RevocationTime: thisUpdate,
				}
			}

			crlBytes, err := caCert.CreateCRL(rand.Reader, priv, revoked, thisUpdate, nextUpdate)
			if err != nil {
				panic(err)
			}
			return crlBytes
		}

		// 1. Store a "new" CRL (ThisUpdate = 2020-06-01)
		t1 := time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)
		crlNew := signCRL(t1, t1.Add(24*time.Hour), []int64{42})

		prev := &CRL{Parent: ds.KeyForObj(ctx, caEntity)}
		crlEntity, err := validateAndStoreCRL(ctx, crlNew, "etag-new", caEntity, prev)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, crlEntity.RevokedCertsCount, should.Equal(1))
		assert.Loosely(t, crlEntity.LastUpdateTime, should.Match(t1))

		// Verify SN 42 is revoked in datastore shards
		checker := NewCRLChecker("Test CA", CRLShardCount, time.Minute)
		revoked, err := checker.IsRevokedSN(ctx, big.NewInt(42))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, revoked, should.BeTrue)

		// 2. Try to store an "old" CRL (ThisUpdate = 2020-01-01)
		t0 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		crlOld := signCRL(t0, t0.Add(24*time.Hour), []int64{}) // no revocations

		_, err = validateAndStoreCRL(ctx, crlOld, "etag-old", caEntity, crlEntity)

		// We expect this to fail.
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, err.Error(), should.ContainSubstring("CRL rollback rejected"))
	})
}
