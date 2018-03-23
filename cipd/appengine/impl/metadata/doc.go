// Copyright 2018 The LUCI Authors.
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

// Package metadata implements handling of prefix metadata.
//
// Each element of the package namespace (known as "package path prefix" or just
// "prefix") can have some metadata attached to it, like Access Control List or
// the prefix description. Such metadata is generally inherited by the packages
// that has the prefix.
//
// The current implementation of PrefixMetadata is based on legacy ACL entities
// produced by Python version of the backend (see legacy.go). It will eventually
// be replaced.
package metadata
