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

"""Defines luci.milo(...) rule."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "keys")

_ALLOWED_STORAGE_RE = r"https://storage\.googleapis\.com/.+"

def _milo(
        ctx,
        *,
        logo = None,
        favicon = None,
        bug_url_template = None,
        monorail_project = None,
        monorail_components = None,
        bug_summary = None,
        bug_description = None):
    r"""Defines optional configuration of the Milo service for this project.

    Milo service is a public user interface for displaying (among other things)
    builds, builders, builder lists (see luci.list_view(...)) and consoles
    (see luci.console_view(...)).

    Can optionally be configured with a bug_url_template for filing bugs via
    custom bug links on build pages.
    The protocol must be `https` and the domain name must be one of the allowed
    domains (see [Project.bug_url_template] for details).

    The template is interpreted as a [mustache] template and the following
    variables are available:
      * {{{ build.builder.project }}}
      * {{{ build.builder.bucket }}}
      * {{{ build.builder.builder }}}
      * {{{ milo_build_url }}}
      * {{{ milo_builder_url }}}

    All variables are URL component encoded. Additionally, use `{{{ ... }}}`
    to disable HTML escaping.
    If the template does not satify the requirements above, the link is not
    displayed.

    [Project.bug_url_template]: https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/milo/api/config/project.proto
    [mustache]: https://mustache.github.io

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      logo: optional https URL to the project logo (usually \*.png), must be
        hosted on `storage.googleapis.com`.
      favicon: optional https URL to the project favicon (usually \*.ico), must
        be hosted on `storage.googleapis.com`.
      bug_url_template: optional string template for making a custom bug link
        for filing a bug against a build that displays on the build page.
      monorail_project: Deprecated. Please use `bug_url_template` instead.
        optional Monorail project to file bugs in when a user clicks the
        feedback link on a build page.
      monorail_components: Deprecated. Please use `bug_url_template` instead.
        a list of the Monorail component to assign to a new bug, in th
         hierarchical `>`-separated format, e.g.
        `Infra>Client>ChromeOS>CI`. Required if `monorail_project` is set,
        otherwise must not be used.
      bug_summary: Deprecated. Please use `bug_url_template` instead.
        string with a text template for generating new bug's summary given a
        builder on whose page a user clicked the bug link. Must not be used if
        `monorail_project` is unset.
      bug_description: Deprecated. Please use `bug_url_template` instead.
        string with a text template for generating new bug's description given a
        builder on whose page a user clicked the bug link. Must not be used if
        `monorail_project` is unset.
    """

    mon_proj = validate.string("monorail_project", monorail_project, required = False)

    mon_comps = validate.list("monorail_components", monorail_components, required = bool(mon_proj))
    for c in mon_comps:
        validate.string("monorail_components", c)

    bug_summ = validate.string("bug_summary", bug_summary, required = False)
    bug_desc = validate.string("bug_description", bug_description, required = False)

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
        "logo": validate.string("logo", logo, regexp = _ALLOWED_STORAGE_RE, required = False),
        "favicon": validate.string("favicon", favicon, regexp = _ALLOWED_STORAGE_RE, required = False),
        "bug_url_template": validate.string("bug_url_template", bug_url_template, required = False),
        "monorail_project": mon_proj,
        "monorail_components": mon_comps,
        "bug_summary": bug_summ,
        "bug_description": bug_desc,
    })
    graph.add_edge(keys.project(), key)

    return graph.keyset(key)

milo = lucicfg.rule(impl = _milo)
