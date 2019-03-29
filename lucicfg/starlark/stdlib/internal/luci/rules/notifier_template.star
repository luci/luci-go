# Copyright 2019 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/lucicfg.star', 'lucicfg')
load('@stdlib//internal/validate.star', 'validate')

load('@stdlib//internal/luci/common.star', 'keys')


def _notifier_template(
      ctx,
      *,
      name=None,
      body=None
  ):
  """Defines a template to use for emails sent by luci.notifier(...).

  The main template body should have format `<subject>\\n\\n<body>` where
  subject is one line of [text/template] and body is an [html/template]. The
  body can either be specified inline right in the starlark script or loaded
  from an external file via io.read_file(...).

  [text/template]: https://godoc.org/text/template
  [html/template]: https://godoc.org/html/template

  #### Template input

  The input to both templates is a structure with fields

  * [Build](https://godoc.org/go.chromium.org/luci/buildbucket/proto#Build):
    a recently completed build for which the email is being generated.
  * [OldStatus](https://godoc.org/go.chromium.org/luci/buildbucket/proto#Status):
    previous status of the builder. Relevant for continuous builders.

  #### Template functions

  The following functions are available to templates in addition to the
  [standard ones](https://godoc.org/text/template#hdr-Functions).

  * `time`: converts a
    [Timestamp](https://godoc.org/github.com/golang/protobuf/ptypes/timestamp#Timestamp)
    to [time.Time](https://godoc.org/time).
    Example: `{{.Build.EndTime | time}}`

  #### Template example

  ```html
  A {{.Build.Builder.Builder}} build completed

  <a href="https://ci.chromium.org/b/{{.Build.Id}}">Build {{.Build.Number}}</a>
  has completed with status {{.Build.Status}}
  on `{{.Build.EndTime | time}}`
  ```

  #### Template sharing

  A template can "import" subtemplates defined in all other
  luci.notifier_template(...). When rendering, *all* templates defined in the
  project are merged into one. Example:

  ```python
  # The actual email template which uses subtemplates defined below. In the real
  # life it might be better to load such large template from an external file
  # using io.read_file.
  luci.notifier_template(
      name = 'default',
      body = '\\n'.join([
          'A {{.Build.Builder.Builder}} completed',
          '',
          'A <a href="https://ci.chromium.org/b/{{.Build.Id}}">build</a> has completed.',
          '',
          'Steps: {{template "steps" .}}',
          '',
          '{{template "footer"}}',
      ]),
  )

  # This template renders only steps. It is "executed" by other templates.
  luci.notifier_template(
      name = 'steps',
      body = '{{range $step := .Build.Steps}}<li>{{$step.name}}</li>{{end}',
  )

  # This template defines subtemplates used by other templates.
  luci.notifier_template(
      name = 'common',
      body = '{{define "footer"}}Have a nice day!{{end}}',
  )
  ```

  #### Error handling

  If a user-defined template fails to render, a built-in template is used to
  generate a very short email with a link to the build and details about the
  failure.

  Args:
    name: name of this template to reference it from luci.notifier(...) rules.
        A template named `default` is used by all notifiers that do not
        explicitly specify another template. Required.
    body: string with the template body. Use io.read_file(...) to load it from
        an external file, if necessary. Required.
  """
  name = validate.string('name', name)
  key = keys.notifier_template(name)
  graph.add_node(key, idempotent = True, props = {
      'name': name,
      'body': validate.string('body', body),
  })
  graph.add_edge(keys.project(), key)
  return graph.keyset(key)


notifier_template = lucicfg.rule(impl = _notifier_template)
