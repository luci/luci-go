// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package signature

import (
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// Check checks blob is signed by the valid signature or not.
// If signature is valid, Check returns nil.
func Check(blob, x509CertPEM, signature []byte) error {
	block, _ := pem.Decode(x509CertPEM)
	if block == nil {
		return fmt.Errorf("x509CertPEM does not contain PEM %v", string(x509CertPEM))
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	d512 := sha512.Sum512(blob)
	return cert.CheckSignature(x509.SHA256WithRSA, d512[:], signature)
}
