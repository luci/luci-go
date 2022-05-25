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
        ctx,  # @unused
        *,
        logo = None,
        favicon = None,
        bug_url_template = None):
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

    All variables are URL component encoded. Additionally, use `{{{ ... }}}` to
    disable HTML escaping. If the template does not satisfy the requirements
    above, the link is not displayed.

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
    """

    key = keys.milo()
    graph.add_node(key, props = {
        "logo": validate.string("logo", logo, regexp = _ALLOWED_STORAGE_RE, required = False),
        "favicon": validate.string("favicon", favicon, regexp = _ALLOWED_STORAGE_RE, required = False),
        "bug_url_template": validate.string("bug_url_template", bug_url_template, required = False),
    })
    graph.add_edge(keys.project(), key)

    return graph.keyset(key)

milo = lucicfg.rule(impl = _milo)
