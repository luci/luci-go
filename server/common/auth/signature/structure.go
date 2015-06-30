// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package signature

// Certificate represents primary's certificate.
type Certificate struct {
	// KeyName represents certificate's name.
	KeyName string `json:"key_name"`
	// X509CertificatePEM represents PEM representation of the certificate.
	X509CertificatePEM string `json:"x509_certificate_pem"`
}

// PublicCertificates is used to decode primary's JSON encoded certificates.
type PublicCertificates struct {
	// Certificates represents the list of certificates.
	Certificates []Certificate `json:"certificates"`
	// Timestamp represents the time list is made by the primary.
	Timestamp int64 `json:"timestamp"`
}
