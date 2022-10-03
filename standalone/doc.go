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

// Package standalone aim to provide a subset of LUCI functionality in a
// standalone environment that does not require any cloud services.
// These services MAY be able to deployed in a cloud setting but should also
// be deployable in a local environment (single host box)
// There are a collection of packages below which are intended to work in
// concert with each other, but may also provide value individually
package standalone
