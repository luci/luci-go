// Copyright 2020 The LUCI Authors.
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

//go:generate cproto -disable-grpc -proto-path . components/auth/proto

package protocol

// Files copied from https://chromium.googlesource.com/infra/luci/luci-py:
//   appengine/components/components/auth/proto/realms.proto
//   appengine/components/components/auth/proto/replication.proto
//   appengine/components/components/auth/proto/security_config.proto
//
// Commit: c2f96a60395ddf9c98ce6ebeba6f456920172262
// Modifications: None
