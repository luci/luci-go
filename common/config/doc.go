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

// Package config is a library to access the luci-config service.
//
// There are three backends for the interface presented in the interface.go.
//
// One backend talks directly to the luci-config service, another reads
// from the local filesystem, and the third one reads from memory struct.
//
// Usually, you should use the remote backend in production, the filesystem
// backend when developing, and the memory backend from unit tests.
package config
