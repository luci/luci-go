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

// Package secrets provides a secrets store based on Google Secret Manager.
//
// It supports stored and auto-generated secrets.
//
// Stored secrets are predefined blobs or texts (e.g. passwords or serialized
// Tink keysets) that are placed in the Google Secret Manager and then read back
// via this package. How secrets are written is outside the scope of this
// package, it just reads them.
//
// Auto-generated secrets are random high-entropy strings with no inherent
// structure. They are usually derived via a key derivation function from their
// name and some root secret read from Google Secret Manager. They are useful
// when calculating HMACs, as salts, etc.
//
// Primary Tink AEAD key
//
// This package exposes functionality to encrypt/decrypt blobs using Tink AEAD
// implementation. The reference to the encryption key in Google Secret Manager
// is supplied via `-primary-tink-aead-key` flag. This key must be explicitly
// initialized first.
//
// Create a new Google Secret Manager secret (with no values) named
// `tink-aead-primary` in the cloud project that the server is running in
// (referred to as `<cloud-project>` below). Make sure the server account has
// "Secret Manager Secret Accessor" role in it. These steps are usually done
// using Terraform.
//
// Initialize the key value by using
// http://go.chromium.org/luci/server/cmd/tink-aead-key tool:
//
//   cd server/cmd/tink-aead-key
//   go run main.go login
//   go run main.go create sm://<cloud-project>/tink-aead-primary
//
// This secret now contains a serialized Tink keyset with the primary encryption
// key. Pass `-primary-tink-aead-key sm://tink-aead-primary` to the server
// binary to tell it to use this key.
//
// If necessary the key can be rotated using the same `tink-aead-key` tool:
//
//   cd server/cmd/tink-aead-key
//   go run main.go rotate sm://<cloud-project>/tink-aead-primary
//
// This will add a new active key to the keyset. It will be used to encrypt
// new plain text values, but the old key will still be recognized when
// decrypting existing cipher texts.
//
// The root secret
//
// The root secret is used to derive auto-generated secrets using HKDF. The
// reference to the root secret key in Google Secret Manager is supplied via
// `-root-secret` flag. This key must be explicitly initialized first.
//
// Create a new Google Secret Manager secret (with no values) named
// `root-secret` in the cloud project that the server is running in (referred to
// as `<cloud-project>` below). Make sure the server account has "Secret Manager
// Secret Accessor" role in it. These steps are usually done using Terraform.
//
// Next generate a sufficiently random string and use it as a new secret value:
//
//   openssl rand -base64 36 | gcloud secrets versions add root-secret \
//       --data-file=- --project <cloud-project>
//
// Pass `-root-secret sm://root-secret` to the server binary to tell it to use
// this key.
//
// To rotate the key, rerun the command above to add a new secret version.
package secrets
