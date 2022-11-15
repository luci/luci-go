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

package rerun

// These constants are used for offsetting the buildbucket run priortity
// The priority ranges from 20 - 255. Lower number means higher priority.
// See go/luci-bisection-run-prioritization for more details.
const (
	// Baseline priority
	PriorityCulpritVerificationHighConfidence   = 100
	PriorityCulpritVerificationMediumConfidence = 120
	PriorityCulpritVerificationLowConfidence    = 140
	PriorityNthSection                          = 130

	// Offset priority
	PriorityTreeClosureOffset                    = -70
	PriorityShortBuildOffset                     = -20
	PriorityMediumBuildOffset                    = -10
	PriorityLongBuildOffset                      = 0
	PriorityVeryLongBuildOffset                  = 40
	PriorityAnalysisTriggeredByNonSheriffsOffset = -5
	PriorityAnalysisTriggeredBySheriffsOffset    = -10
	PriorityFlakyBuilderOffset                   = 50
	PriorityAnotherVerificationBuildExistOffset  = 20
	PrioritySuspectInMultipleFailedBuildOffset   = -20
	PrioritySuspectIsRevertedOffset              = 25
	PriorityNthSectionNoSuspectOffset            = -10
	PriorityScheduleOnSameBotOffset              = -15
)

// CapPriority caps priority into the range [20..255]
func CapPriority(priority int32) int32 {
	if priority < 20 {
		return 20
	}
	if priority > 255 {
		return 255
	}
	return priority
}
