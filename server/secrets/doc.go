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
// # Primary Tink AEAD key
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
// https://go.chromium.org/luci/server/cmd/secret-tool tool:
//
//	cd server/cmd/secret-tool
//	go run main.go create sm://<cloud-project>/tink-aead-primary -secret-type tink-aes256-gcm
//
// This secret now contains a serialized Tink keyset with the primary encryption
// key. Pass `-primary-tink-aead-key sm://tink-aead-primary` to the server
// binary to tell it to use this key.
//
// If necessary the key can be rotated using the same `secret-tool` tool:
//
//	cd server/cmd/secret-tool
//	go run main.go rotation-begin sm://<cloud-project>/tink-aead-primary
//	# wait several hours for the new key to propagate into all caches
//	# confirm by looking at /chrome/infra/secrets/gsm/version metric
//	go run main.go rotation-end sm://<cloud-project>/tink-aead-primary
//
// This will add a new active key to the keyset. It will be used to encrypt
// new plain text values, but the old key will still be recognized when
// decrypting existing cipher texts until the next rotation occurs: at most one
// old key version is retained currently.
//
// # The root secret
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
// Next create a new secret version with a randomly generated blob:
//
//	cd server/cmd/secret-tool
//	go run main.go create sm://<cloud-project>/root-secret -secret-type random-bytes-32
//
// Pass `-root-secret sm://root-secret` to the server binary to tell it to use
// this key.
//
// To rotate the key use the `secret-tool` again:
//
//	cd server/cmd/secret-tool
//	go run main.go rotation-begin sm://<cloud-project>/root-secret
//	# wait several hours for the new key to propagate into all caches
//	go run main.go rotation-end sm://<cloud-project>/root-secret
package secrets
