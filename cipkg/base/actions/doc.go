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

// Package actions includes:
// 1. a set of transformers for default cipkg actions
//   - core.ActionCommand
//   - core.ActionFilesCopy
//   - core.ActionCIPDExport
//   - core.ActionURLFetch
//
// 2. ActionProcessor and Transformer for converting Action to Derivation and
// registering user defined action.
package actions
