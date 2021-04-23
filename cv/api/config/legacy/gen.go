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

//go:generate cproto

// Package tricium has a simplified version of Tricium Project Config proto.
//
// Note that some proto messages are modified for simplicity reason as long as
// it can generate the same text proto as the original Tricium Project Config
// proto does:
// https://chromium.googlesource.com/infra/infra/+/HEAD/go/src/infra/tricium/api/v1
package tricium
