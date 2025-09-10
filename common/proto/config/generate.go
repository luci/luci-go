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

// Package config contains luci-config protobuf definitions.
//
// Copied from:
//
// Repo: https://chromium.googlesource.com/infra/luci/luci-py/
// Revision: 857db7b19c3765e1dd44432fe2bffb1450070e3c
// Path: appengine/components/components/config/proto/*.proto
//
// Modification: added luci.* options.
package config

//go:generate cproto -use-grpc-plugin -use-ancient-protoc-gen-go
