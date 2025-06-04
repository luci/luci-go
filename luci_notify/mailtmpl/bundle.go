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
	"context"
	"fmt"
	html "html/template"
	"strings"
	text "text/template"
	"time"

	"github.com/russross/blackfriday/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/data/text/sanitizehtml"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/luci_notify/api/config"
)

const (
	// FileExt is a file extension of template files.
	FileExt = ".template"

	// DefaultTemplateName of the default template.
	DefaultTemplateName = "default"
)

// Funcs is functions available to email subject and body templates.
var Funcs = map[string]any{
	"time": func(ts *timestamppb.Timestamp) time.Time {
		t := ts.AsTime()
		return t
	},

	"formatBuilderID": protoutil.FormatBuilderID,

	// markdown renders the given text as markdown HTML.
	//
	// This uses blackfriday to convert from markdown to HTML,
	// and sanitizehtml to allow only a small subset of HTML through.
	"markdown": func(inputMD string) html.HTML {
		// We don't want auto punctuation, which changes "foo" into “foo”
		r := blackfriday.NewHTMLRenderer(blackfriday.HTMLRendererParameters{
			Flags: blackfriday.UseXHTML,
		})
		untrusted := blackfriday.Run(
			[]byte(inputMD),
			blackfriday.WithRenderer(r),
			blackfriday.WithExtensions(
				blackfriday.NoIntraEmphasis|
					blackfriday.FencedCode|
					blackfriday.Autolink,
				// TODO(tandrii): support Tables, which are currently sanitized away by
				// sanitizehtml.Sanitize.
			))
		out := bytes.NewBuffer(nil)
		if err := sanitizehtml.Sanitize(out, bytes.NewReader(untrusted)); err != nil {
			return html.HTML(fmt.Sprintf("Failed to render markdown: %s", html.HTMLEscapeString(err.Error())))
		}
		return html.HTML(out.String())
	},

	"stepNames": func(steps []*buildbucketpb.Step) string {
		var sb strings.Builder
		for i, step := range steps {
			if i != 0 {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "%q", step.Name)
		}

		return sb.String()
	},

	"buildUrl": func(input *config.TemplateInput) string {
		return fmt.Sprintf("https://%s/build/%d",
			input.BuildbucketHostname, input.Build.Id)
	},
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
			errs = append(errs, errors.Fmt("template %q: %w", t.Name, err))
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

// GenerateStatusMessage generates a message to be posted to a tree status instance.
// If the template fails, a default template is used.
func (b *Bundle) GenerateStatusMessage(c context.Context, templateName string, input *config.TemplateInput) (message string) {
	var err error
	if message, _, err = b.executeUserTemplate(templateName, input); err != nil {
		logging.Errorf(c, "Template %q failed to render: %s", templateName, err)
		message = generateDefaultStatusMessage(input)
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

	errorTemplateInput := map[string]any{
		"Build":               input.Build,
		"BuildbucketHostname": input.BuildbucketHostname,
		"TemplateName":        templateName,
		"TemplateURL":         "",
		"Error":               err.Error(),
	}
	if t := b.templates[templateName]; t != nil {
		errorTemplateInput["TemplateURL"] = t.DefinitionURL
	}

	var buf bytes.Buffer
	if err := errorBodyTemplate.Execute(&buf, errorTemplateInput); err != nil {
		// Error template MAY NOT fail.
		panic(errors.Fmt("execution of the error template has failed: %w", err))
	}
	body = buf.String()
	return
}

const defaultStatusTemplateStr = "{{ stepNames .MatchingFailedSteps }} on {{ buildUrl . }} {{ .Build.Builder.Builder }}{{ if .Build.Input.GitilesCommit }} from {{ .Build.Input.GitilesCommit.Id }}{{end}}"

var defaultStatusTemplate *text.Template = text.Must(text.New("").Funcs(Funcs).Parse(defaultStatusTemplateStr))

func generateDefaultStatusMessage(input *config.TemplateInput) string {
	var buf bytes.Buffer
	if err := defaultStatusTemplate.Execute(&buf, input); err != nil {
		panic(errors.Fmt("execution of the default status message template has failed: %w", err))
	}

	return buf.String()
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
	case len(parts) == 1:
		return strings.TrimSpace(parts[0]), "", nil

	case len(strings.TrimSpace(parts[1])) > 0:
		return "", "", fmt.Errorf("second line is not blank: %q", parts[1])

	case len(parts) == 2:
		// In this case the second line must be blank, because of the
		// check above, so we're just dropping the blank line.
		return strings.TrimSpace(parts[0]), "", nil

	default:
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[2]), nil
	}
}
