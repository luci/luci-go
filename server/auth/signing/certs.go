// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package signing

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/internal"
	"github.com/luci/luci-go/server/proccache"
)

// Certificate is public certificate of some service. Must not be mutated once
// initialized.
type Certificate struct {
	// KeyName identifies the key used for signing.
	KeyName string `json:"key_name"`
	// X509CertificatePEM is PEM encoded certificate.
	X509CertificatePEM string `json:"x509_certificate_pem"`
}

// PublicCertificates is a bundle of recent certificates of some service. Must
// not be mutated once initialized.
type PublicCertificates struct {
	// Certificates is the list of certificates.
	Certificates []Certificate `json:"certificates"`
	// Timestamp is Unix time (microseconds) of when this list was generated.
	Timestamp JSONTime `json:"timestamp"`

	lock  sync.Mutex
	cache map[string]*x509.Certificate
}

// JSONTime is time.Time that serializes as unix timestamp (in microseconds).
type JSONTime time.Time

// Time casts value to time.Time.
func (t JSONTime) Time() time.Time {
	return time.Time(t)
}

// UnmarshalJSON implements json.Unmarshaler.
func (t *JSONTime) UnmarshalJSON(data []byte) error {
	ts, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return err
	}
	*t = JSONTime(time.Unix(0, ts*1000))
	return nil
}

// MarshalJSON implements json.Marshaler.
func (t JSONTime) MarshalJSON() ([]byte, error) {
	ts := t.Time().UnixNano() / 1000
	return []byte(strconv.FormatInt(ts, 10)), nil
}

type proccacheKey string

// FetchCertificates fetches certificates from given URL. The server is expected
// to reply with JSON described by PublicCertificates struct (like LUCI services
// do). Uses proccache to cache them for 1 hour.
//
// LUCI services serve certificates at /auth/api/v1/server/certificates.
func FetchCertificates(c context.Context, url string) (*PublicCertificates, error) {
	certs, err := proccache.GetOrMake(c, proccacheKey(url), func() (interface{}, time.Duration, error) {
		certs := &PublicCertificates{}
		err := internal.FetchJSON(c, certs, func() (*http.Request, error) {
			return http.NewRequest("GET", url, nil)
		})
		if err != nil {
			return nil, 0, err
		}
		return certs, time.Hour, nil
	})
	if err != nil {
		return nil, err
	}
	return certs.(*PublicCertificates), nil
}

// CertificateForKey finds the certificate for given key and deserializes it.
func (pc *PublicCertificates) CertificateForKey(key string) (*x509.Certificate, error) {
	pc.lock.Lock()
	defer pc.lock.Unlock()
	if cert, ok := pc.cache[key]; ok {
		return cert, nil
	}
	for _, cert := range pc.Certificates {
		if cert.KeyName == key {
			block, _ := pem.Decode([]byte(cert.X509CertificatePEM))
			if block == nil {
				return nil, fmt.Errorf("signature: the certificate %q is not PEM encoded", key)
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, err
			}
			if pc.cache == nil {
				pc.cache = make(map[string]*x509.Certificate)
			}
			pc.cache[key] = cert
			return cert, nil
		}
	}
	return nil, fmt.Errorf("signature: no such certificate %q", key)
}

// CheckSignature returns nil if `signed` was indeed signed by given key.
func (pc *PublicCertificates) CheckSignature(key string, signed, signature []byte) error {
	cert, err := pc.CertificateForKey(key)
	if err != nil {
		return err
	}
	return cert.CheckSignature(x509.SHA256WithRSA, signed, signature)
}
