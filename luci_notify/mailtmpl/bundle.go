// Copyright 2019 The LUCI Authors.
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

// Package mailtmpl implements email template bundling and execution.
package mailtmpl

import (
	"bytes"
	"fmt"
	html "html/template"
	"strings"
	text "text/template"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/luci_notify/api/config"
)

const (
	// FileExt is a file extension of template files.
	FileExt = ".template"

	// DefaultTemplateName of the default template.
	DefaultTemplateName = "default"
)

// Funcs is functions available to email subject and body templates.
var Funcs = map[string]interface{}{
	"time": func(ts *timestamp.Timestamp) time.Time {
		t, _ := ptypes.Timestamp(ts)
		return t
	},
	"formatBuilderID": protoutil.FormatBuilderID,
}

// Template is an email template.
// To render it, use NewBundle.
type Template struct {
	// Name identifies the email template. It is unique within a bundle.
	Name string

	// SubjectTextTemplate is a text.Template of the email subject.
	// See Funcs for available functions.
	SubjectTextTemplate string

	// BodyHTMLTemplate is a html.Template of the email body.
	// See Funcs for available functions.
	BodyHTMLTemplate string

	// URL to the template definition.
	// Will be used in template error reports.
	DefinitionURL string
}

// Bundle is a collection of email templates bundled together, so they
// can use each other.
type Bundle struct {
	// Error found among templates.
	// If non-nil, GenerateEmail will generate error emails.
	Err error

	templates map[string]*Template
	subjects  *text.Template
	bodies    *html.Template
}

// NewBundle bundles templates together and makes them renderable.
// If templates do not have a template "default", bundles in one.
// May return a bundle with an non-nil Err.
func NewBundle(templates []*Template) *Bundle {
	b := &Bundle{
		subjects:  text.New("").Funcs(Funcs),
		bodies:    html.New("").Funcs(Funcs),
		templates: make(map[string]*Template, len(templates)+1),
	}

	addTemplate := func(t *Template) error {
		if _, err := b.subjects.New(t.Name).Parse(t.SubjectTextTemplate); err != nil {
			return err
		}

		_, err := b.bodies.New(t.Name).Parse(t.BodyHTMLTemplate)
		return err
	}

	var errs errors.MultiError

	hasDefault := false
	for _, t := range templates {
		if _, ok := b.templates[t.Name]; ok {
			errs = append(errs, fmt.Errorf("duplicate template %q", t.Name))
		}
		b.templates[t.Name] = t

		if t.Name == DefaultTemplateName {
			hasDefault = true
		}
		if err := addTemplate(t); err != nil {
			errs = append(errs, errors.Annotate(addTemplate(t), "template %q", t.Name).Err())
		}
	}

	if !hasDefault {
		if err := addTemplate(defaultTemplate); err != nil {
			panic(err)
		}
	}

	if len(errs) > 0 {
		b.Err = errs
	}

	return b
}

// GenerateEmail generates an email using the named template. If the template
// fails, an error template is used, which includes error details and a link to
// the definition of the failed template.
func (b *Bundle) GenerateEmail(templateName string, input *config.TemplateInput) (subject, body string) {
	var err error
	if subject, body, err = b.executeUserTemplate(templateName, input); err != nil {
		// Execution of the user-defined template failed.
		// Fallback to the error template.
		subject, body = b.generateErrorEmail(templateName, input, err)
	}
	return
}

// executeUserTemplate executed a user-defined template.
func (b *Bundle) executeUserTemplate(templateName string, input *config.TemplateInput) (subject, body string, err error) {
	var buf bytes.Buffer
	if err = b.subjects.ExecuteTemplate(&buf, templateName, input); err != nil {
		return
	}
	subject = buf.String()

	buf.Reset()
	if err = b.bodies.ExecuteTemplate(&buf, templateName, input); err != nil {
		return
	}
	body = buf.String()
	return
}

// generateErrorEmail generates a spartan email that contains information
// about an error during execution of a user-defined template.
func (b *Bundle) generateErrorEmail(templateName string, input *config.TemplateInput, err error) (subject, body string) {
	subject = fmt.Sprintf(`[Build Status] Builder %q`, protoutil.FormatBuilderID(input.Build.Builder))

	errorTemplateInput := map[string]interface{}{
		"Build":        input.Build,
		"TemplateName": templateName,
		"TemplateURL":  "",
		"Error":        err.Error(),
	}
	if t := b.templates[templateName]; t != nil {
		errorTemplateInput["TemplateURL"] = t.DefinitionURL
	}

	var buf bytes.Buffer
	if err := errorBodyTemplate.Execute(&buf, errorTemplateInput); err != nil {
		// Error template MAY NOT fail.
		panic(errors.Annotate(err, "execution of the error template has failed").Err())
	}
	body = buf.String()
	return
}

// SplitTemplateFile splits an email template file into subject and body.
// Does not validate their syntaxes.
// See notify.proto for file format.
func SplitTemplateFile(content string) (subject, body string, err error) {
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
