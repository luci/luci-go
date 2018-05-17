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
	"bytes"
	"fmt"
	html "html/template"
	"regexp"
	"strings"
	text "text/template"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	configInterface "go.chromium.org/luci/config"
)

// emailTemplateFilenameRegexp is a regular expression for email template file
// names.
// The first path component is a app id. We do not validate it here.
var emailTemplateFilenameRegexp = regexp.MustCompile(`^[^/]+/email-templates/([a-z][a-z0-9_]*)\.template$`)

// ParseEmailTemplate parses email subject and body templates.
func ParseEmailTemplate(subject, body string) (parsedSubject *text.Template, parsedBody *html.Template, err error) {
	parsedSubject, err = text.New("subject").Parse(subject)
	if err != nil {
		return nil, nil, err // error includes template name
	}

	parsedBody, err = html.New("body").Parse(body)
	// Due to luci-config limitation, we cannot detect an invalid reference to
	// a sub-template defined in a different file.
	if err != nil {
		return nil, nil, err // error includes template name
	}

	return
}

// EmailTemplate is a Datastore entity directly under Project entity that
// represents an email template.
// It is managed by the cron job that ingests configs.
type EmailTemplate struct {
	// ProjectKey is a datastore key of the LUCI project containing this email
	// template.
	ProjectKey *datastore.Key `gae:"$parent"`

	// Name identifies the email template. It is unique within the project.
	Name string `gae:"$id"`

	// SubjectTextTemplate is a text.Template of the email subject.
	SubjectTextTemplate string

	// BodyHTMLTemplate is a html.Template of the email body.
	BodyHTMLTemplate string
}

// EmailTemplateBundle combines all email body templates into one
// html.Template such that one template can reuse parts of another.
// Use (*EmailTemplateBundle).Parse to make one and use
// (*EmailTemplateBundle).Execute to render an email.
type EmailTemplateBundle struct {
	// subjects is a mapping from template name ot the subject template.
	subjects map[string]*text.Template

	// bodies is a single html.Template that combines all body templates.
	bodies *html.Template
}

// Bundle bundles email templates into EmailTemplateBundle.
// Returns an error if template names are not unique or templates are invalid.
func (b *EmailTemplateBundle) Parse(templates []*EmailTemplate) error {
	b.subjects = make(map[string]*text.Template, len(templates))
	b.bodies = html.New("")

	for _, t := range templates {
		if _, ok := b.subjects[t.Name]; ok {
			return errors.Reason("duplicate template name %q", t.Name).Err()
		}

		subject, body, err := ParseEmailTemplate(t.SubjectTextTemplate, t.BodyHTMLTemplate)
		if err != nil {
			return errors.Annotate(err, "invalid template %s", t.Name).Err()
		}

		b.subjects[t.Name] = subject

		// Insert body and its subtemplates into the common b.bodies.
		for _, st := range body.Templates() {
			treeName := st.Name()
			if st == body {
				treeName = t.Name
			}
			if _, err := b.bodies.AddParseTree(treeName, st.Tree); err != nil {
				// This cannot happen because AddParseTree may return an error only if the
				// template was executed before. It was not.
				panic(err)
			}
		}
	}

	return nil
}

// Execute executes a named email template and returned rendered subject and body.
func (b *EmailTemplateBundle) Execute(templateName string, data interface{}) (subject, body string, err error) {
	subjectTemplate, ok := b.subjects[templateName]
	if !ok {
		return "", "", errors.Reason("unknown template %q", templateName).Err()
	}

	var buf bytes.Buffer
	if err := subjectTemplate.Execute(&buf, data); err != nil {
		return "", "", errors.Annotate(err, "failed to execute subject template").Err()
	}
	subject = buf.String()

	buf.Reset()
	if err := b.bodies.ExecuteTemplate(&buf, templateName, data); err != nil {
		return "", "", errors.Annotate(err, "failed to execute body template").Err()
	}
	body = buf.String()
	return
}

// fetchAllEmailTemplates fetches all valid email templates of the project.
// Returned EmailTemplate entities do not have ProjectKey set.
func fetchAllEmailTemplates(c context.Context, configService configInterface.Interface, projectId string) (map[string]*EmailTemplate, error) {
	configSet := configInterface.ProjectSet(projectId)
	files, err := configService.ListFiles(c, configSet)
	if err != nil {
		return nil, err
	}

	ret := map[string]*EmailTemplate{}

	appIdPrefix := info.AppID(c) + "/"
	// This runs in a cron job. It is not performance critical, so we don't have
	// to fetch files concurrently.
	for _, f := range files {
		m := emailTemplateFilenameRegexp.FindStringSubmatch(f)
		// We have to check appid prefix in addition to the regexp because
		// regexp does not check appid.
		if m == nil || !strings.HasPrefix(f, appIdPrefix) {
			// Not a template file or a template of another instance of luci-notify.
			continue
		}
		templateName := m[1]

		logging.Infof(c, "fetching email template from %s:%s", configSet, f)
		config, err := configService.GetConfig(c, configSet, f, false)
		if err != nil {
			return nil, errors.Annotate(err, "failed to fetch %q", f).Err()
		}

		subject, body, err := splitEmailTemplateFile(config.Content)
		if err != nil {
			// Should not happen. luci-config should not have passed this commit in
			// because luci-notify exposes its validation code to luci-conifg.
			logging.Warningf(c, "invalid email template content in %q: %s", f, err)
			continue
		}

		ret[templateName] = &EmailTemplate{
			Name:                templateName,
			SubjectTextTemplate: subject,
			BodyHTMLTemplate:    body,
		}
	}
	return ret, nil
}

// splitEmailTemplateFile splits an email template file into subject and body.
// Does not validate their syntaxes.
// See notify.proto for file format.
func splitEmailTemplateFile(content string) (subject, body string, err error) {
	if len(content) == 0 {
		return "", "", fmt.Errorf("empty file")
	}

	parts := strings.SplitN(content, "\n", 3)
	switch {
	case len(parts) < 3:
		return "", "", fmt.Errorf("less than three lines")

	case len(strings.TrimSpace(parts[1])) > 0:
		return "", "", fmt.Errorf("second line is not blank: %q", parts[1])

	default:
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[2]), nil
	}
}
