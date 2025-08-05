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

package bugs

import (
	"fmt"
	"regexp"
)

const (
	SourceBugRuleUpdatedTemplate = `Because this bug was merged into another bug, LUCI Analysis has` +
		` merged the failure association rule for this bug into the rule for the canonical bug.

See failure impact and configure the failure association rule for the canoncial bug at: %s`

	LinkTemplate = `See failure impact and configure the failure association rule for this bug at: %s`

	NoPermissionTemplate = `This bug was filed in the fallback component instead of in component %d because LUCI Analysis does not have permissions to that component.`

	ComponentArchivedTemplate = `This bug was filed in the fallback component instead of in component %d because that component is archived.`

	BugTitlePrefix = "Tests are failing: "
)

// whitespaceRE matches blocks of whitespace, including new lines tabs and
// spaces.
var whitespaceRE = regexp.MustCompile(`[ \t\n]+`)

// GenerateBugSummary generates a unified form of bug summary with a sanitized title.
func GenerateBugSummary(title string) string {
	return fmt.Sprintf("%s%v", BugTitlePrefix, sanitiseTitle(title, 150))
}

// sanitiseTitle removes tabs and line breaks from input, replacing them with
// spaces, and truncates the output to the given number of runes.
func sanitiseTitle(input string, maxLength int) string {
	// Replace blocks of whitespace, including new lines and tabs, with just a
	// single space.
	strippedInput := whitespaceRE.ReplaceAllString(input, " ")

	// Truncate to desired length.
	runes := []rune(strippedInput)
	if len(runes) > maxLength {
		return string(runes[0:maxLength-3]) + "..."
	}
	return strippedInput
}
