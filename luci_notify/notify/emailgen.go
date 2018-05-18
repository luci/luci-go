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

package notify

import (
	"bytes"
	"context"
	"fmt"
	html "html/template"
	"strings"
	text "text/template"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/luci_notify/config"
)

var templateFuncs = html.FuncMap{
	"time": func(ts *timestamp.Timestamp) time.Time {
		t, _ := ptypes.Timestamp(ts)
		return t
	},
}

// errorBodyTemplate is used when a user-defined email body template fails.
var errorBodyTemplate = html.Must(html.New("error").Funcs(templateFuncs).Parse(strings.TrimSpace(`
<p>A <a href="{{ .Build.ViewUrl }}">build</a>
  on builder <code>{{ .Build.Builder.IDString }}</code>
  completed with status <code>{{.Build.Status}}</code>.</p>

<p>This email is so spartan because the actual
<a href="{{.TemplateURL}}">email template <code>{{.TemplateName}}</code></a>
failed to execute on this build:
<pre>{{.Error}}</pre>
</p>
`)))

// TODO(nodir): start requiring a default template in the config and delete
// this.
var defaultTemplate = config.EmailTemplate{
	Name:                "default",
	SubjectTextTemplate: `[Build Status] Builder "{{ .Build.Builder.IDString }}"`,
	BodyHTMLTemplate: `luci-notify detected a status change for builder "{{ .Build.Builder.IDString }}"
at {{ .Build.EndTime | time }}.

<table>
  <tr>
    <td>New status:</td>
    <td><b>{{ .Build.Status }}</b></td>
  </tr>
  <tr>
    <td>Previous status:</td>
    <td>{{ .OldStatus }}</td>
  </tr>
  <tr>
    <td>Builder:</td>
    <td>{{ .Build.Builder.IDString }}</td>
  </tr>
  <tr>
    <td>Created by:</td>
    <td>{{ .Build.CreatedBy }}</td>
  </tr>
  <tr>
    <td>Created at:</td>
    <td>{{ .Build.CreateTime | time }}</td>
  </tr>
  <tr>
    <td>Finished at:</td>
    <td>{{ .Build.EndTime | time }}</td>
  </tr>
</table>

<a href="{{ .Build.ViewUrl }}">Full details are available here.</a><br/><br/>

You are receiving the default template as no template was provided or a template
name did not match the one provided.`,
}

// bundle is a collection of email tempaltes bundled together.
type bundle struct {
	revision string
	subjects *text.Template
	bodies   *html.Template
	err      error
	defURLs  map[string]string
}

type emailTemplateInput struct {
	*buildbucketpb.Build
	OldStatus buildbucketpb.Status
}

// GenerateEmail generates an email using the named template. If the template
// fails, an error template is used, which includes error details and a link to
// the definition of the email template that has failed.
func (b *bundle) GenerateEmail(templateName string, input *emailTemplateInput) (subject, body string) {
	var err error
	subject, body, err = b.executeUserTemplate(templateName, input)
	if err != nil {
		subject, body = b.executeErrorTemplate(templateName, err, input)
	}
	return
}

// executeUserTemplate executed a named template.
// If b.err is not nil, returns it right away.
// Execute executes a named email template and returns rendered subject and body.
func (b *bundle) executeUserTemplate(templateName string, input *emailTemplateInput) (subject, body string, err error) {
	if b.err != nil {
		err = b.err
		return
	}

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

// executeErrorTemplate generates a spartan email that contains information
// about an error during execution of a user-defined template.
func (b *bundle) executeErrorTemplate(templateName string, err error, input *emailTemplateInput) (subject, body string) {
	// Execution of the user-defined template failed.
	// Fallback to the error template.
	subject = fmt.Sprintf(`[Build Status] Builder %q`, input.Build.Builder.IDString())

	errorTemplateInput := map[string]interface{}{
		"Build":        input.Build,
		"TemplateName": templateName,
		"TemplateURL":  b.defURLs[templateName],
		"Error":        err.Error(),
	}
	var buf bytes.Buffer
	if err := errorBodyTemplate.Execute(&buf, errorTemplateInput); err != nil {
		// This may not happen.
		panic(errors.Annotate(err, "execution of the error template has failed").Err())
	}
	body = buf.String()
	return
}

var templateCache = caching.RegisterLRUCache(128)

// getBundle returns a bundle of email templates for the given project.
// The returned bundle is cached in the process memory, do not modify it.
//
// Returns an error only on transient failures.
func getBundle(c context.Context, projectId string) (*bundle, error) {
	// Fetch current revision of the project config.
	project := &config.Project{Name: projectId}
	if err := datastore.Get(c, project); err != nil {
		return nil, errors.Annotate(err, "failed to fetch project").Err()
	}

	var transientErr error
	// Lookup an exising bundle in the process cache.
	// If not available, compile and cache.
	value, ok := templateCache.LRU(c).Mutate(c, projectId, func(it *lru.Item) *lru.Item {
		if it.Value != nil && it.Value.(*bundle).revision == project.Revision {
			return it // Cache hit.
		}

		// Cache miss. Either no cached value or revision mismatch.

		// Fetch all templates from the Datastore transactionally with the project.
		// On a transient error, return it and do not purge cache.
		var templates []*config.EmailTemplate
		transientErr = datastore.RunInTransaction(c, func(c context.Context) error {
			if err := datastore.Get(c, project); err != nil {
				return err
			}

			q := datastore.NewQuery("EmailTemplate").Ancestor(datastore.KeyForObj(c, project))
			return datastore.GetAll(c, q, &templates)
		}, nil)
		if transientErr != nil {
			return it
		}

		// Legacy: add a default template if we don't have one.
		// TODO(nodir): delete this once all projects define their templates.
		hasDefault := false
		for _, t := range templates {
			if t.Name == defaultTemplate.Name {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			templates = append(templates, &defaultTemplate)
		}

		// Bundle all fetched templates. If bundling/parsing fails, cache the error,
		// so we don't recompile bad templates over and over.
		b := &bundle{
			revision: project.Revision,
			subjects: text.New(""),
			bodies:   html.New(""),
			defURLs:  make(map[string]string, len(templates)),
		}
		for _, t := range templates {
			b.defURLs[t.Name] = t.DefinitionURL
			if _, b.err = b.subjects.New(t.Name).Parse(t.SubjectTextTemplate); b.err != nil {
				break
			}
			if _, b.err = b.bodies.New(t.Name).Parse(t.BodyHTMLTemplate); b.err != nil {
				break
			}
		}

		// Cache without expiration.
		return &lru.Item{Value: b}
	})

	switch {
	case transientErr != nil:
		return nil, transientErr
	case !ok:
		panic("impossible: no cached value and no error")
	default:
		return value.(*bundle), nil
	}
}
