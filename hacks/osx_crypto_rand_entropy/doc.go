// Copyright 2022 The LUCI Authors.
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

// Package osx_crypto_rand_entropy, when linked into a binary, replaces
// getentropy with a crashing stub, and makes crypto/rand use the older way
// to source entropy, based on reading /dev/urandom.
//
// getentropy is absent on OSX 10.11 and earlier, but >=go1.17 links to it.
// This hack allows to run binaries compiled with go1.17 on OSX 10.11.
package osx_crypto_rand_entropy
