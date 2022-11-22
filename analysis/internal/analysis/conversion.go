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

package analysis

import (
	"strings"

	pb "go.chromium.org/luci/analysis/proto/v1"
)

// ToBQBuildStatus converts a luci.analysis.v1.BuildStatus to its BigQuery
// column representation. This trims the BUILD_STATUS_ prefix to avoid
// excessive verbosity in the table.
func ToBQBuildStatus(value pb.BuildStatus) string {
	return strings.TrimPrefix(value.String(), "BUILD_STATUS_")
}

// FromBQBuildStatus extracts luci.analysis.v1.BuildStatus from
// its BigQuery column representation.
func FromBQBuildStatus(value string) pb.BuildStatus {
	return pb.BuildStatus(pb.BuildStatus_value["BUILD_STATUS_"+value])
}

// ToBQPresubmitRunStatus converts a luci.analysis.v1.PresubmitRunStatus to its
// BigQuery column representation. This trims the PRESUBMIT_RUN_STATUS_ prefix
// to avoid excessive verbosity in the table.
func ToBQPresubmitRunStatus(value pb.PresubmitRunStatus) string {
	return strings.TrimPrefix(value.String(), "PRESUBMIT_RUN_STATUS_")
}

// FromBQPresubmitRunStatus extracts luci.analysis.v1.PresubmitRunStatus from
// its BigQuery column representation.
func FromBQPresubmitRunStatus(value string) pb.PresubmitRunStatus {
	return pb.PresubmitRunStatus(pb.PresubmitRunStatus_value["PRESUBMIT_RUN_STATUS_"+value])
}

// ToBQPresubmitRunMode converts a luci.analysis.v1.PresubmitRunMode to its
// BigQuery column representation.
func ToBQPresubmitRunMode(value pb.PresubmitRunMode) string {
	return value.String()
}

// FromBQPresubmitRunMode extracts luci.analysis.v1.PresubmitRunMode from
// its BigQuery column representation.
func FromBQPresubmitRunMode(value string) pb.PresubmitRunMode {
	return pb.PresubmitRunMode(pb.PresubmitRunMode_value[value])
}

// FromBQExonerationReason extracts luci.analysis.v1.ExonerationReason from
// its BigQuery column representation.
func FromBQExonerationReason(value string) pb.ExonerationReason {
	return pb.ExonerationReason(pb.ExonerationReason_value[value])
}

// FromBQChangelistOwnershipKind extracts luci.analysis.v1.ChangelistOwnerKind
// from its BigQuery column representation.
func FromBQChangelistOwnershipKind(value string) pb.ChangelistOwnerKind {
	return pb.ChangelistOwnerKind(pb.ChangelistOwnerKind_value[value])
}
