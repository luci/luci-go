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
// It supports stored and auto-generated secrets. Stored secrets are predefined
// blobs (e.g. passwords) that can placed in the Google Secret Manager and then
// can be read back via this package.
//
// Auto-generated secrets are random high-entropy strings with no inherent
// structure derived via a key derivation function from their name and some
// root secret. They are useful when calculating hashes, as salts, etc.
package secrets
