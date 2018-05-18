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

// emailTemplateFilenameRegexp returns a regular expression for email template
// file names.
func emailTemplateFilenameRegexp(c context.Context) *regexp.Regexp {
	appId := info.AppID(c)
	pattern := fmt.Sprintf(`^%s+/email-templates/([a-z][a-z0-9_]*)\.template$`, regexp.QuoteMeta(appId))
	return regexp.MustCompile(pattern)
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
	SubjectTextTemplate string `gae:",noindex"`

	// BodyHTMLTemplate is a html.Template of the email body.
	BodyHTMLTemplate string `gae:",noindex"`

	// DefinitionURL is a URL to human-viewable page that contains the definition
	// of this email template.
	DefinitionURL string `gae:",noindex"`
}

// EmailTemplateBundle is a collection of email templates. It combines all email
// body templates into one html.Template such that one template can reuse
// another.
type EmailTemplateBundle struct {
	subjects *text.Template
	bodies   *html.Template
}

// Add adds t to the bundle.
func (b *EmailTemplateBundle) Add(t *EmailTemplate) error {
	if b.subjects == nil {
		b.subjects = text.New("")
	}
	if _, err := b.subjects.New(t.Name).Parse(t.SubjectTextTemplate); err != nil {
		return errors.Annotate(err, "invalid subject template %s", t.Name).Err()
	}

	if b.bodies == nil {
		b.bodies = html.New("")
	}
	if _, err := b.bodies.New(t.Name).Parse(t.BodyHTMLTemplate); err != nil {
		return errors.Annotate(err, "invalid body template %s", t.Name).Err()
	}

	return nil
}

// Has returns true is the named template exists in b.
func (b *EmailTemplateBundle) Has(name string) bool {
	return b.bodies.Lookup(name) != nil
}

// Execute executes a named email template and returns rendered subject and body.
func (b *EmailTemplateBundle) Execute(templateName string, data interface{}) (subject, body string, err error) {
	var buf bytes.Buffer
	if err := b.subjects.ExecuteTemplate(&buf, templateName, data); err != nil {
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

// FetchEmailTemplateBundle fetches from Datastore, parses and bundles all email
// templates of a project.
func FetchEmailTemplateBundle(c context.Context, projectId string) (*EmailTemplateBundle, error) {
	var bundle EmailTemplateBundle
	q := datastore.NewQuery("EmailTemplate").Ancestor(datastore.KeyForObj(c, &Project{Name: projectId}))
	err := datastore.Run(c, q, func(t *EmailTemplate) error {
		return bundle.Add(t)
	})
	if err != nil {
		return nil, err
	}
	return &bundle, nil
}

// fetchAllEmailTemplates fetches all valid email templates of the project from
// a config service. Returned EmailTemplate entities do not have ProjectKey set.
func fetchAllEmailTemplates(c context.Context, configService configInterface.Interface, projectId string) (map[string]*EmailTemplate, error) {
	configSet := configInterface.ProjectSet(projectId)
	files, err := configService.ListFiles(c, configSet)
	if err != nil {
		return nil, err
	}

	ret := map[string]*EmailTemplate{}

	// This runs in a cron job. It is not performance critical, so we don't have
	// to fetch files concurrently.
	filenameRegexp := emailTemplateFilenameRegexp(c)
	for _, f := range files {
		m := filenameRegexp.FindStringSubmatch(f)
		if m == nil {
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
			DefinitionURL:       config.ViewURL,
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
