// Copyright 2017 The LUCI Authors.
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

package iam

import "context"

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
