// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package signing

import (
	"golang.org/x/net/context"
)

// Signer holds private key and correspond cert and can sign blobs with
// PKCS1v15.
type Signer interface {
	// SignBytes signs the blob with some active private key.
	// Returns the signature and name of the key used.
	SignBytes(c context.Context, blob []byte) (keyName string, signature []byte, err error)

	// Certificates returns a bundle with public certificates for all active keys.
	Certificates(c context.Context) (*PublicCertificates, error)
}
