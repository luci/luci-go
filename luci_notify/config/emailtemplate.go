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
	"context"
	"fmt"
	"regexp"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	configInterface "go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/luci_notify/common"
	"go.chromium.org/luci/luci_notify/mailtmpl"
)

// emailTemplateFilenameRegexp returns a regular expression for email template
// file names.
func emailTemplateFilenameRegexp(c context.Context) (*regexp.Regexp, error) {
	appID, err := common.GetAppID(c)
	if err != nil {
		return nil, errors.Fmt("failed to get app ID: %w", err)
	}

	return regexp.MustCompile(fmt.Sprintf(
		`^%s+/email-templates/([a-z][a-z0-9_]*)%s$`,
		regexp.QuoteMeta(appID),
		regexp.QuoteMeta(mailtmpl.FileExt),
	)), nil
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

// Template converts t to *mailtmpl.Template.
func (t *EmailTemplate) Template() *mailtmpl.Template {
	return &mailtmpl.Template{
		Name:                t.Name,
		SubjectTextTemplate: t.SubjectTextTemplate,
		BodyHTMLTemplate:    t.BodyHTMLTemplate,
		DefinitionURL:       t.DefinitionURL,
	}
}

// fetchAllEmailTemplates fetches all valid email templates of the project from
// a config service. Returned EmailTemplate entities do not have ProjectKey set.
func fetchAllEmailTemplates(c context.Context, configService configInterface.Interface, projectID string) (map[string]*EmailTemplate, error) {
	configSet, err := configInterface.ProjectSet(projectID)
	if err != nil {
		return nil, err
	}
	files, err := configService.ListFiles(c, configSet)
	if err != nil {
		return nil, err
	}

	ret := map[string]*EmailTemplate{}

	// This runs in a cron job. It is not performance critical, so we don't have
	// to fetch files concurrently.
	filenameRegexp, err := emailTemplateFilenameRegexp(c)
	if err != nil {
		return nil, err
	}
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
			return nil, errors.Fmt("failed to fetch %q: %w", f, err)
		}

		subject, body, err := mailtmpl.SplitTemplateFile(config.Content)
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
