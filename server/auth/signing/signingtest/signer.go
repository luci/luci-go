// Copyright 2015 The LUCI Authors.
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

package signingtest

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"time"

	"go.chromium.org/luci/server/auth/signing"
)

// Signer holds private key and corresponding cert and can sign blobs with
// PKCS1v15.
type Signer struct {
	priv        *rsa.PrivateKey
	certs       signing.PublicCertificates
	serviceInfo signing.ServiceInfo
}

var _ signing.Signer = (*Signer)(nil)

// NewSigner returns Signer instance that use small random key.
//
// Panics on errors.
func NewSigner(serviceInfo *signing.ServiceInfo) *Signer {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		panic(err)
	}

	// Generate fake self signed certificate.
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Unix(1000000000, 0),
		NotAfter:              time.Unix(10000000000, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		panic(err)
	}

	// PEM-encode it, derive the name.
	pemOut := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyName := sha256.Sum256(der)

	// Put in default service info if necessary.
	if serviceInfo == nil {
		serviceInfo = &signing.ServiceInfo{}
	}

	return &Signer{
		priv: priv,
		certs: signing.PublicCertificates{
			AppID:              serviceInfo.AppID,
			ServiceAccountName: serviceInfo.ServiceAccountName,
			Certificates: []signing.Certificate{
				{
					KeyName:            hex.EncodeToString(keyName[:20]),
					X509CertificatePEM: string(pemOut),
				},
			},
			Timestamp: signing.JSONTime(time.Unix(1000000000, 0)),
		},
		serviceInfo: *serviceInfo,
	}
}

// SignBytes signs the blob with some active private key.
// Returns the signature and name of the key used.
func (s *Signer) SignBytes(ctx context.Context, blob []byte) (keyName string, signature []byte, err error) {
	// Passing nil as 'rand' to SignPKCS1v15 disables RSA blinding, making
	// the signature deterministic, but reduces overall security. Do not do it
	// in non testing code.
	digest := sha256.Sum256(blob)
	sig, err := rsa.SignPKCS1v15(nil, s.priv, crypto.SHA256, digest[:])
	return s.certs.Certificates[0].KeyName, sig, err
}

// Certificates returns a bundle with public certificates for all active keys.
func (s *Signer) Certificates(ctx context.Context) (*signing.PublicCertificates, error) {
	return &s.certs, nil
}

// ServiceInfo returns information about the current service.
//
// It includes app ID and the service account name (that ultimately owns the
// signing private key).
func (s *Signer) ServiceInfo(ctx context.Context) (*signing.ServiceInfo, error) {
	return &s.serviceInfo, nil
}

// KeyForTest returns the RSA key used internally by the test signer.
//
// It is not part of the signing.Signer interface. It should be used only from
// tests.
func (s *Signer) KeyForTest() *rsa.PrivateKey {
	return s.priv
}

// KeyNameForTest returns an ID of the signing key.
func (s *Signer) KeyNameForTest() string {
	return s.certs.Certificates[0].KeyName
}
