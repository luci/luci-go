// Copyright 2021 The LUCI Authors.
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

// Package cvpb contains v0 (preliminary) version of CV API.
//
// !!!!! WARNING !!!!!
//   - Use at your own risk.
//   - We will stop supporting this v0 API without notice.
//   - No backwards compatibility guaranteed.
//   - Please, contact CV maintainers at luci-eng@ before using this and
//     we may provide additional guarantees to you/your service.
package cvpb

//go:generate cproto -use-grpc-plugin -enable-pgv
