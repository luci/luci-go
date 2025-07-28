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

package resultpb

// UpdateTokenMetadataKey is the metadata.MD key for the secret update token
// required to mutate a root invocation, work unit or invocation.
// It is returned by CreateRootInvocation, CreateWorkUnit and CreateInvocation
// RPCs in response header metadata, and is required by all RPCs mutating a
// root invocation, work unit or invocation.
const UpdateTokenMetadataKey = "update-token"

// InclusionTokenMetadataKey is the metadata.MD key for the secret inclusion token
// required to include a work unit into another work unit.
// It is returned by DelegateWorkUnitInclusion RPC in response header metadata,
// and is an alternative to providing an "update-token" to CreateWorkUnit RPC.
const InclusionTokenMetadataKey = "inclusion-token"
