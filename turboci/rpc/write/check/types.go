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

// Kind is a shorthand equivalent to [orchestratorpb.CheckKind].
type Kind = orchestratorpb.CheckKind

// These are shorthand equivalents to orchestratorpb.CheckKind_CHECK_KIND_*.
const (
	KindUnknown  Kind = orchestratorpb.CheckKind_CHECK_KIND_UNKNOWN
	KindAnalysis Kind = orchestratorpb.CheckKind_CHECK_KIND_ANALYSIS
	KindBuild    Kind = orchestratorpb.CheckKind_CHECK_KIND_BUILD
	KindSource   Kind = orchestratorpb.CheckKind_CHECK_KIND_SOURCE
	KindTest     Kind = orchestratorpb.CheckKind_CHECK_KIND_TEST
)

// State is a shorthand equivalent to [orchestratorpb.CheckState].
type State = orchestratorpb.CheckState

// These are shorthand equivalents to orchestratorpb.CheckState_CHECK_STATE_*.
const (
	StateUnknown  State = orchestratorpb.CheckState_CHECK_STATE_UNKNOWN
	StatePlanned  State = orchestratorpb.CheckState_CHECK_STATE_PLANNED
	StatePlanning State = orchestratorpb.CheckState_CHECK_STATE_PLANNING
	StateWaiting  State = orchestratorpb.CheckState_CHECK_STATE_WAITING
	StateFinal    State = orchestratorpb.CheckState_CHECK_STATE_FINAL
)
