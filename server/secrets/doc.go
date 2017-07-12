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

// Package secrets provides an interface for a simple secret store: you ask it
// for a secret (a byte blob, identifies by some key), and it returns it
// to you (current version, as well as a bunch of previous versions). Caller are
// supposed to use the secret for an operation and then forget it (e.g. do not
// try to store it elsewhere).
//
// Secure storage, retrieval and rotation of secrets is outside of the scope of
// this interface: it's the responsibility of the implementation.
package secrets
