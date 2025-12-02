// Copyright 2025 The LUCI Authors.
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

package check

import orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

// Kind is a shorthand equivalent to orchestratorpb.CheckKind.
type Kind = orchestratorpb.CheckKind

// These are shorthand equivalents of their
// orchestratorpb.CheckKind_CHECK_KIND_* counterparts.
const (
	KindUnknown  Kind = orchestratorpb.CheckKind_CHECK_KIND_UNKNOWN
	KindSource   Kind = orchestratorpb.CheckKind_CHECK_KIND_SOURCE
	KindBuild    Kind = orchestratorpb.CheckKind_CHECK_KIND_BUILD
	KindTest     Kind = orchestratorpb.CheckKind_CHECK_KIND_TEST
	KindAnalysis Kind = orchestratorpb.CheckKind_CHECK_KIND_ANALYSIS
)

// State is a shorthand equivalent to orchestratorpb.CheckState.
type State = orchestratorpb.CheckState

// These are shorthand equivalents of their
// orchestratorpb.CheckKind_CHECK_STATE_* counterparts.
const (
	StateUnknown  State = orchestratorpb.CheckState_CHECK_STATE_UNKNOWN
	StatePlanning State = orchestratorpb.CheckState_CHECK_STATE_PLANNING
	StatePlanned  State = orchestratorpb.CheckState_CHECK_STATE_PLANNED
	StateWaiting  State = orchestratorpb.CheckState_CHECK_STATE_WAITING
	StateFinal    State = orchestratorpb.CheckState_CHECK_STATE_FINAL
)
