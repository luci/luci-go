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

package policy

import (
	"fmt"

	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/clustering"
)

const NewBugTrailingDescription = `This bug has been automatically filed by LUCI Analysis in response to a cluster of test failures.`

// NewIssueDescriptionLegacy generates the description that should
// be used when the issue is first created.
// It includes the given threshold comment, which can justify:
// * why the bug has its initial priority; or
// * why the bug was automatically filed.
// It adds information about actioning the bug and what to do
// if the component is not correct.
func NewIssueDescriptionLegacy(description *clustering.ClusterDescription, uiBaseURL, thresholdComment, ruleLink string) string {
	var bodies []string
	bodies = append(bodies, description.Description)
	if ruleLink != "" {
		bodies = append(bodies, fmt.Sprintf("See failure impact and configure the failure association rule for this bug at: %s", ruleLink))
	}
	bodies = append(bodies, thresholdComment)
	bodies = append(bodies, NewBugTrailingDescription)

	footers := []string{
		fmt.Sprintf("How to action this bug: %s", BugFiledHelpURL(uiBaseURL)),
		fmt.Sprintf("Provide feedback: %s", FeedbackURL(uiBaseURL)),
		fmt.Sprintf("Was this bug filed in the wrong component? See: %s", ComponentSelectionHelpURL(uiBaseURL)),
	}

	return bugs.Commentary{
		Bodies:  bodies,
		Footers: footers,
	}.ToComment()
}
