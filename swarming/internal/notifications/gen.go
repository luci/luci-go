// Copyright 2023 The LUCI Authors.
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

// Package notifications contains protos for RBE Reservations notifications.
//
// Protos are copied from http://shortn/_brt6pXcfS0.
//
// Changes:
//
// 1. Change `go_package` to match the new location.
// 2. Some comments stripped.
package notifications

//go:generate cproto -use-grpc-plugin -discovery=false
