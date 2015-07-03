// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build appengine

package signature

import (
	"crypto/sha512"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/appengine"
)

// PublicCerts returns list of public certificates.
func PublicCerts(c context.Context) (*PublicCertificates, error) {
	aeCerts, err := appengine.PublicCertificates(c)
	if err != nil {
		return nil, err
	}
	var certs []Certificate
	for _, ac := range aeCerts {
		certs = append(certs, Certificate{
			KeyName:            ac.KeyName,
			X509CertificatePEM: string(ac.Data),
		})
	}
	return &PublicCertificates{
		Certificates: certs,
		Timestamp:    time.Now().Unix(),
	}, nil
}

// Sign returns a signing key name and a signature.
func Sign(c context.Context, blob []byte) (keyName string, signature []byte, err error) {
	d512 := sha512.Sum512(blob)
	return appengine.SignBytes(c, d512[:])
}
