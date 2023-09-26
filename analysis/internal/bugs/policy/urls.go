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

import "fmt"

func RuleURL(uiBaseURL, project, ruleID string) string {
	return fmt.Sprintf("%s/p/%s/rules/%s", uiBaseURL, project, ruleID)
}

func BugFiledHelpURL(uiBaseURL string) string {
	return fmt.Sprintf("%s/help#new-bug-filed", uiBaseURL)
}

func FeedbackURL(uiBaseURL string) string {
	return fmt.Sprintf("%s/help#feedback", uiBaseURL)
}

func ComponentSelectionHelpURL(uiBaseURL string) string {
	return fmt.Sprintf("%s/help#component-selection", uiBaseURL)
}

func BugVerifiedHelpURL(uiBaseURL string) string {
	return fmt.Sprintf("%s/help#bug-verified", uiBaseURL)
}

func BugReopenedHelpURL(uiBaseURL string) string {
	return fmt.Sprintf("%s/help#bug-reopened", uiBaseURL)
}

func PriorityUpdatedHelpURL(uiBaseURL string) string {
	return fmt.Sprintf("%s/help#priority-updated", uiBaseURL)
}

func BuganizerBugRuleURL(uiBaseURL string, issueID int64) string {
	return fmt.Sprintf("%s/b/%d", uiBaseURL, issueID)
}
