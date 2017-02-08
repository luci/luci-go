// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package iam

import (
	"golang.org/x/net/context"
)

// Signer implements SignBytes interface on top of IAM client.
//
// It signs blobs using some service account's private key via 'signBlob' IAM
// call.
type Signer struct {
	Client         *Client
	ServiceAccount string
}

// SignBytes signs the blob with some active private key.
//
// Hashes the blob using SHA256 and then calculates RSASSA-PKCS1-v1_5 signature
// using the currently active signing key.
//
// Returns the signature and name of the key used.
func (s *Signer) SignBytes(c context.Context, blob []byte) (string, []byte, error) {
	return s.Client.SignBlob(c, s.ServiceAccount, blob)
}
