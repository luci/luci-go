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

package bugs

import (
	"strings"
	"text/template"

	"go.chromium.org/luci/common/errors"
)

// templateMaxOutputBytes is the maximum number of bytes that a template should
// produce. This limit is enforced strictly at config validation time and
// loosely (actual size may be 2x larger) at runtime.
const templateMaxOutputBytes = 10_000

// Template is a template for bug comments.
type Template struct {
	template *template.Template
}

// ParseTemplate parses the given template source.
func ParseTemplate(t string) (Template, error) {
	templ, err := template.New("comment_template").Parse(t)
	if err != nil {
		return Template{}, err
	}
	return Template{templ}, nil
}

// Execute executes the template.
func (t *Template) Execute(input TemplateInput) (string, error) {
	var b strings.Builder
	err := t.template.Execute(&b, input)
	if err != nil {
		return "", errors.Fmt("execute: %w", err)
	}
	if b.Len() > 2*templateMaxOutputBytes {
		return "", errors.Fmt("template produced %v bytes of output, which exceeds the limit of %v bytes", b.Len(), 2*templateMaxOutputBytes)
	}
	return b.String(), nil
}

// Validate validates the template.
func (t Template) Validate() error {
	type testCase struct {
		name  string
		input TemplateInput
	}
	testCases := []testCase{
		{
			name: "buganizer",
			input: TemplateInput{
				RuleURL: "https://luci-analysis-deployment/some/url",
				BugID: TemplateBugID{
					id: BugID{
						System: BuganizerSystem,
						ID:     "1234567890123",
					},
				},
			},
		}, {
			name: "monorail",
			input: TemplateInput{
				RuleURL: "https://luci-analysis-deployment/some/url",
				BugID: TemplateBugID{
					id: BugID{
						System: MonorailSystem,
						ID:     "monorailproject/1234567890123",
					},
				},
			},
		},
		{
			// Reserve the ability to extend to other bug-filing systems; the
			// template should handle this gracefully.
			name: "neither buganizer nor monorail",
			input: TemplateInput{
				RuleURL: "https://luci-analysis-deployment/some/url",
				BugID: TemplateBugID{
					id: BugID{
						System: "reserved",
						ID:     "reserved",
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		var b strings.Builder
		err := t.template.Execute(&b, tc.input)
		if err != nil {
			return errors.Fmt("test case %q: %w", tc.name, err)
		}
		if b.Len() > templateMaxOutputBytes {
			return errors.Fmt("test case %q: template produced %v bytes of output, which exceeds the limit of %v bytes", tc.name, b.Len(), templateMaxOutputBytes)
		}
	}
	return nil
}

// TemplateInput is the input to the policy-specified template for generating
// bug comments.
type TemplateInput struct {
	// The link to the LUCI Analysis failure association rule.
	RuleURL string
	// The identifier of the bug on which we are commenting.
	BugID TemplateBugID
}

// NewTemplateBugID initializes a new TemplateBugID.
func NewTemplateBugID(id BugID) TemplateBugID {
	return TemplateBugID{id: id}
}

// TemplateBugID wraps the BugID type so we do not couple the interface the
// seen by a project's bug template to our implementation details.
// We want full control over the interface the template sees to ensure
// project configuration compatibility over time.
type TemplateBugID struct {
	// must remain private.
	id BugID
}

// IsBuganizer returns whether the bug is a Buganizer bug.
func (b TemplateBugID) IsBuganizer() bool {
	return b.id.System == BuganizerSystem
}

// IsMonorail returns whether the bug is a monorail bug.
func (b TemplateBugID) IsMonorail() bool {
	return b.id.System == MonorailSystem
}

// MonorailProject returns the monorail project for a bug.
// (e.g. "chromium" for crbug.com/123456).
// Errors if the bug is not a monorail bug.
func (b TemplateBugID) MonorailProject() (string, error) {
	project, _, err := b.id.MonorailProjectAndID()
	return project, err
}

// MonorailBugID returns the monorail ID for a bug
// (e.g. "123456" for crbug.com/123456).
// Errors if the bug is not a monorail bug.
func (b TemplateBugID) MonorailBugID() (string, error) {
	_, id, err := b.id.MonorailProjectAndID()
	return id, err
}

// BuganizerBugID returns the buganizer ID for a bug.
// E.g. "123456" for "b/123456".
// Errors if the bug is not a buganizer bug.
func (b TemplateBugID) BuganizerBugID() (string, error) {
	if b.id.System != BuganizerSystem {
		return "", errors.New("not a buganizer bug")
	}
	return b.id.ID, nil
}
