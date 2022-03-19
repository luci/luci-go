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

// Package provenance contains the protos (defining all the possible provenance
// information messages which are supported) and a simple library interface
// which lets you report provenance.
//
// Summary
//
// Provenance defines APIs for reporting `provenance` metadata, in order to
// establish a traceable lineage for artifacts produced within the LUCI ecosystem.
// Artifact provenance is otherwise defined as *"metadata which records a snapshot
// of the build-time states that correspond to an artifact."*
//
// TODO(akashmukherjee): update this doc.
package provenance
