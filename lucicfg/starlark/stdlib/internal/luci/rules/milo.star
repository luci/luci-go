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


_ALLOWED_STORAGE_RE = r'https://storage\.googleapis\.com/.+'


def _milo(
      ctx,
      *,
      logo=None,
      favicon=None,
      monorail_project=None,
      monorail_components=None,
      bug_summary=None,
      bug_description=None
  ):
  """Defines optional configuration of the Milo service for this project.

  Milo service is a public user interface for displaying (among other things)
  builds, builders, builder lists (see luci.list_view(...)) and consoles
  (see luci.console_view(...)).

  Can optionally be configured with a reference to a [Monorail] project to use
  for filing bugs via custom bug links on build pages. The format of a new bug
  is defined via `bug_summary` and `bug_description` fields which are
  interpreted as Golang [text templates]. They can either be given directly as
  strings, or loaded from external files via io.read_file(...).

  Supported interpolations are the fields of the standard build proto such as:

      {{.Build.Builder.Project}}
      {{.Build.Builder.Bucket}}
      {{.Build.Builder.Builder}}

  Other available fields include:

      {{.MiloBuildUrl}}
      {{.MiloBuilderUrl}}

  If any specified placeholder cannot be satisfied then the bug link is not
  displayed.

  [Monorail]: https://bugs.chromium.org
  [text templates]: https://golang.org/pkg/text/template

  Args:
    logo: optional https URL to the project logo, must be hosted on
        `storage.googleapis.com`.
    favicon: optional https URL to the project favicon, must be hosted on
        `storage.googleapis.com`.
    monorail_project: optional Monorail project to file bugs in when a user
        clicks the feedback link on a build page.
    monorail_components: a list of the Monorail component to assign to a new
        bug, in the hierarchical `>`-separated format, e.g.
        `Infra>Client>ChromeOS>CI`. Required if `monorail_project` is set,
        otherwise must not be used.
    bug_summary: string with a text template for generating new bug's summary
        given a builder on whose page a user clicked the bug link. Must not be
        used if `monorail_project` is unset.
    bug_description: string with a text template for generating new bug's
        description given a builder on whose page a user clicked the bug link.
        Must not be used if `monorail_project` is unset.
  """
  mon_proj = validate.string('monorail_project', monorail_project, required=False)

  mon_comps = validate.list('monorail_components', monorail_components, required=bool(mon_proj))
  for c in mon_comps:
    validate.string('monorail_components', c)

  bug_summ = validate.string('bug_summary', bug_summary, required=False)
  bug_desc = validate.string('bug_description', bug_description, required=False)

  # Monorail-related fields make sense only if monorail_project is given.
  if not mon_proj:
    if mon_comps:
      fail('"monorail_components" are ignored if "monorail_project" is not set')
    if bug_summ:
      fail('"bug_summary" is ignored if "monorail_project" is not set')
    if bug_desc:
      fail('"bug_description" is ignored if "monorail_project" is not set')

  key = keys.milo()
  graph.add_node(key, props = {
      'logo': validate.string('logo', logo, regexp=_ALLOWED_STORAGE_RE, required=False),
      'favicon': validate.string('favicon', favicon, regexp=_ALLOWED_STORAGE_RE, required=False),
      'monorail_project': mon_proj,
      'monorail_components': mon_comps,
      'bug_summary': bug_summ,
      'bug_description': bug_desc,
  })
  graph.add_edge(keys.project(), key)

  return graph.keyset(key)


milo = lucicfg.rule(impl = _milo)
