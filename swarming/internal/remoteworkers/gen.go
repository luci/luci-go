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

// Package remoteworkers contains protos for RBE Reservations API.
//
// Protos are copied from http://shortn/_mcfgni4dHh at revision 690693583.
//
// Changes:
//
// 1. Remove unsupported protoc options.
// 2. Change `go_package` to match the new location.
// 3. Change `worker.proto` import path to match the new location.
// 4. Some comments stripped.
package remoteworkers

//go:generate cproto -use-grpc-plugin -discovery=false
