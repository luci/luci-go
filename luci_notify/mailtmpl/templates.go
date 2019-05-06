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
	html "html/template"
	"strings"
)

// errorBodyTemplate is used when a user-defined email template fails.
var errorBodyTemplate = html.Must(html.New("error").
	Funcs(Funcs).
	Parse(strings.TrimSpace(`
<p>A <a href="https://{{.BuildbucketHostname}}/build/{{.Build.Id}}">build</a>
	on builder <code>{{ .Build.Builder | formatBuilderID }}</code>
	completed with status <code>{{.Build.Status}}</code>.</p>

<p>This email is so spartan because the actual
<a href="{{.TemplateURL}}">email template <code>{{.TemplateName}}</code></a>
has failed on this build:
<pre>{{.Error}}</pre>
</p>
`)))

var defaultTemplate = &Template{
	Name:                DefaultTemplateName,
	SubjectTextTemplate: `[Build Status] Builder "{{ .Build.Builder | formatBuilderID }}"`,
	BodyHTMLTemplate: strings.TrimSpace(`
luci-notify detected a status change for builder "{{ .Build.Builder | formatBuilderID }}"
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
    <td>{{ .Build.Builder | formatBuilderID }}</td>
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

Full details are available
<a href="https://{{.BuildbucketHostname}}/build/{{.Build.Id}}">here</a>.
<br/><br/>

You are receiving the default template as no template was provided or a template
name did not match the one provided.
`),
}
