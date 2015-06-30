// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package signature

// X509CertByName returns proper certificate based on the name.
// Note that the returned value is PEM encoded.
func X509CertByName(pc *PublicCertificates, key string) []byte {
	if pc == nil {
		return nil
	}
	for _, cert := range pc.Certificates {
		if cert.KeyName == key {
			return []byte(cert.X509CertificatePEM)
		}
	}
	return nil
}
