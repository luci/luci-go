// Copyright 2018 The LUCI Authors.
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

package config

import (
	"fmt"
	html "html/template"
	"regexp"
	"strings"
	text "text/template"
)

// emailTemplateFilenameRegexp is a regular expression for email template file
// names.
// The first path component is a app id. We do not validate it here.
var emailTemplateFilenameRegexp = regexp.MustCompile(`^[^/]+/email-templates/[a-z][a-z0-9_]*\.template$`)

// ParsedEmailTemplate is a parsed email template file.
type ParsedEmailTemplate struct {
	Subject *text.Template
	Body    *html.Template
}

// Parse parses an email template file contents.
// See notify.proto for file format.
func (t *ParsedEmailTemplate) Parse(content string) error {
	if len(content) == 0 {
		return fmt.Errorf("empty file")
	}

	parts := strings.SplitN(content, "\n", 3)
	switch {
	case len(parts) < 3:
		return fmt.Errorf("less than three lines")

	case len(strings.TrimSpace(parts[1])) > 0:
		return fmt.Errorf("second line is not blank: %q", parts[1])

	default:
		subject, err := text.New("subject").Parse(strings.TrimSpace(parts[0]))
		if err != nil {
			return err // error includes template name
		}

		body, err := html.New("body").Parse(strings.TrimSpace(parts[2]))
		// Due to luci-config limitation, we cannot detect an invalid reference to
		// a sub-template defined in a different file.
		if err != nil {
			return err // error includes template name
		}

		t.Subject = subject
		t.Body = body
		return nil
	}
}
