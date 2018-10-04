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

package signing

import "context"

// Signer holds private key and corresponding cert and can sign blobs.
//
// It signs using RSA-256 + PKCS1v15. It usually lives in a context, see
// GetSigner function.
type Signer interface {
	// SignBytes signs the blob with some active private key.
	//
	// Hashes the blob using SHA256 and then calculates RSASSA-PKCS1-v1_5
	// signature using the currently active signing key.
	//
	// Returns the signature and name of the key used.
	SignBytes(c context.Context, blob []byte) (keyName string, signature []byte, err error)

	// Certificates returns a bundle with public certificates for all active keys.
	//
	// The certificates contains public keys that can be used to validate
	// signatures generated with SignBytes. See CheckSignature method of
	// PublicCertificates.
	//
	// Do not modify return value, it may be shared by many callers.
	Certificates(c context.Context) (*PublicCertificates, error)

	// ServiceInfo returns information about the current service.
	//
	// It includes app ID and the service account name (that ultimately owns the
	// signing private key).
	//
	// Do not modify return value, it may be shared by many callers.
	ServiceInfo(c context.Context) (*ServiceInfo, error)
}
