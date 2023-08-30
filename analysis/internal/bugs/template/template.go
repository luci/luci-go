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

// Package template constructs comments for bug filing policies.
package template

import (
	"bytes"
	"text/template"

	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/common/errors"
)

// Template is a template for bug comments.
type Template struct {
	template *template.Template
}

// Parse parses the given template source.
func Parse(t string) (*Template, error) {
	templ, err := template.New("comment_template").Parse(t)
	if err != nil {
		return nil, err
	}
	return &Template{templ}, nil
}

// Validate validates the template.
func (t *Template) Validate() error {
	type testCase struct {
		name  string
		input *TemplateInput
	}
	testCases := []testCase{
		{
			name: "buganizer",
			input: &TemplateInput{
				RuleURL: "https://luci-analysis-deployment/some/url",
				BugID: BugID{
					id: bugs.BugID{
						System: bugs.BuganizerSystem,
						ID:     "1234567890123",
					},
				},
			},
		}, {
			name: "monorail",
			input: &TemplateInput{
				RuleURL: "https://luci-analysis-deployment/some/url",
				BugID: BugID{
					id: bugs.BugID{
						System: bugs.MonorailSystem,
						ID:     "monorailproject/1234567890123",
					},
				},
			},
		},
		{
			// Reserve the ability to extend to other bug-filing systems; the
			// template should handle this gracefully.
			name: "neither buganizer nor monorail",
			input: &TemplateInput{
				RuleURL: "https://luci-analysis-deployment/some/url",
				BugID: BugID{
					id: bugs.BugID{
						System: "reserved",
						ID:     "reserved",
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		var b bytes.Buffer
		err := t.template.Execute(&b, tc.input)
		if err != nil {
			return errors.Annotate(err, "test case %q", tc.name).Err()
		}
		if b.Len() > 10000 {
			return errors.Reason("test case %q: template produced more than 10,000 bytes of output", tc.name).Err()
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
	BugID BugID
}

// BugID wraps the bugs.BugID type so we do not couple the interface the
// seen by a project's bug template to our implementation details.
// We want full control over the interface the template sees to ensure
// project configuration compatibility over time.
type BugID struct {
	// must remain private.
	id bugs.BugID
}

// IsBuganizer returns whether the bug is a Buganizer bug.
func (b *BugID) IsBuganizer() bool {
	return b.id.System == bugs.BuganizerSystem
}

// IsMonorail returns whether the bug is a monorail bug.
func (b *BugID) IsMonorail() bool {
	return b.id.System == bugs.MonorailSystem
}

// MonorailProject returns the monorail project for a bug.
// (e.g. "chromium" for crbug.com/123456).
// Errors if the bug is not a monorail bug.
func (b *BugID) MonorailProject() (string, error) {
	project, _, err := b.id.MonorailProjectAndID()
	return project, err
}

// MonorailBugID returns the monorail ID for a bug
// (e.g. "123456" for crbug.com/123456).
// Errors if the bug is not a monorail bug.
func (b *BugID) MonorailBugID() (string, error) {
	_, id, err := b.id.MonorailProjectAndID()
	return id, err
}

// BuganizerBugID returns the buganizer ID for a bug.
// E.g. "123456" for "b/123456".
// Errors if the bug is not a buganizer bug.
func (b *BugID) BuganizerBugID() (string, error) {
	if b.id.System != bugs.BuganizerSystem {
		return "", errors.New("not a buganizer bug")
	}
	return b.id.ID, nil
}
