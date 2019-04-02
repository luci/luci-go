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

package buildbucket

// This file contains helper functions for pb package.
// TODO(nodir): move existing helpers from pb to this file.

// BuildTokenHeader is the name of gRPC metadata header indicating the build
// token (see BuildSecrets.BuildToken).
// It is required in UpdateBuild RPC.
// Defined in
// https://chromium.googlesource.com/infra/infra/+/c189064/appengine/cr-buildbucket/v2/api.py#35
const BuildTokenHeader = "x-build-token"
