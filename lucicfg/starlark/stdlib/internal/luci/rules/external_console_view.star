# Copyright 2020 The LUCI Authors.
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

"""Defines luci.external_console_view(...) rule."""

load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "keys", "view")

def _external_console_view(
        ctx,  # @unused
        *,
        name = None,
        title = None,
        source = None):
    """Includes a Milo console view from another project.

    This console will be listed in the Milo UI on the project page, alongside
    the consoles native to this project.

    In the following example, we include a console from the 'chromium' project
    called 'main', and we give it a local name of 'cr-main' and title of
    'Chromium Main Console'.

        luci.external_console_view(
            name = 'cr-main',
            title = 'Chromium Main Console',
            source = 'chromium:main'
        )

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      name: a local name for this console. Will be used for sorting consoles on
        the project page. Note that the name must not clash with existing
        consoles or list views in this project. Required.
      title: a title for this console, will show up in UI. Defaults to `name`.
      source: a string referring to the external console to be included, in the
        format `project:console_id`. Required.
    """
    chunks = validate.string("source", source, regexp = r"^[^:]+:[^:]+$").split(":", 1)
    external_project, external_id = chunks[0], chunks[1]

    return view.add_view(
        key = keys.external_console_view(name),
        entry_kind = None,
        entry_ctor = None,
        entries = [],
        props = {
            "name": validate.string("name", name),
            "title": validate.string("title", title, default = name, required = False),
            "external_project": external_project,
            "external_id": external_id,
        },
    )

external_console_view = lucicfg.rule(impl = _external_console_view)
