// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Unlike signer_appengine.go, signer.go is provided for TEST PURPOSE ONLY.
// That is why RSA key length is far from secure.

// +build !appengine

package signature

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"sync"
	"time"

	"golang.org/x/net/context"
)

const (
	testCertKeyLength     = 512
	testCertValidDuration = time.Hour
)

var (
	// serialNumber is a hardcoded serial number.
	// Our verifier does not use this value, but it is needed to
	// generate a certificate.
	serialNumber = big.NewInt(1)

	muTestCerts sync.Mutex
	testCerts   *testCertificates
)

type testCertificates struct {
	pubCerts *PublicCertificates
	priv     *rsa.PrivateKey
	keyName  string
	err      error
}

// keyName generates keyName from pem.
// It just calcualtes sha256 of pem, and outputs with hex string.
func keyName(pem []byte) string {
	s := sha256.Sum256(pem)
	return hex.EncodeToString(s[:])[:8]
}

// generateCerts generates testCertificates for given keyLength and duration.
func generateCerts(keyLength int, duration time.Duration) *testCertificates {
	priv, err := rsa.GenerateKey(rand.Reader, keyLength)
	if err != nil {
		return &testCertificates{err: err}
	}
	notBefore := time.Now()
	notAfter := notBefore.Add(duration)

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return &testCertificates{err: err}
	}
	pemOut := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	k := keyName(pemOut)
	return &testCertificates{
		pubCerts: &PublicCertificates{
			Certificates: []Certificate{
				{
					KeyName:            k,
					X509CertificatePEM: string(pemOut),
				},
			},
			Timestamp: notBefore.Unix(),
		},
		priv:    priv,
		keyName: k,
	}
}

func globalTestCerts() *testCertificates {
	muTestCerts.Lock()
	defer muTestCerts.Unlock()
	if testCerts != nil {
		return testCerts
	}
	testCerts = generateCerts(testCertKeyLength, testCertValidDuration)
	return testCerts
}

// SetTestCerts is an API to set certificate.
func SetTestCerts(pc *PublicCertificates, priv *rsa.PrivateKey, keyName string) {
	muTestCerts.Lock()
	defer muTestCerts.Unlock()
	testCerts = &testCertificates{
		pubCerts: pc,
		priv:     priv,
		keyName:  keyName,
	}
}

// PublicCerts returns list of public certificates.
func PublicCerts(c context.Context) (*PublicCertificates, error) {
	tc := globalTestCerts()
	return tc.pubCerts, tc.err
}

// Sign returns a signing key name and a signature.
func Sign(c context.Context, blob []byte) (string, []byte, error) {
	tc := globalTestCerts()
	if tc.err != nil {
		return "", nil, tc.err
	}
	d512 := sha512.Sum512(blob)
	d256 := sha256.Sum256(d512[:])
	sig, err := rsa.SignPKCS1v15(rand.Reader, tc.priv, crypto.SHA256, d256[:])
	return tc.keyName, sig, err
}
