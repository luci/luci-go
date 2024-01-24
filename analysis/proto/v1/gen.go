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

package analysispb

//go:generate cproto
//go:generate svcdec -type ClustersServer
//go:generate svcdec -type MetricsServer
//go:generate svcdec -type ProjectsServer
//go:generate svcdec -type RulesServer
//go:generate svcdec -type TestHistoryServer
//go:generate svcdec -type TestVariantsServer
//go:generate svcdec -type BuganizerTesterServer
//go:generate svcdec -type TestVariantBranchesServer
//go:generate svcdec -type ChangepointsServer
