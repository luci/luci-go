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
load('@stdlib//internal/validate.star', 'validate')

load('@stdlib//internal/luci/common.star', 'keys', 'kinds')


def console_view_entry(
      builder=None,
      short_name=None,
      category=None,
      console_view=None,
      buildbot=None,
  ):
  """A builder entry in some luci.console_view(...).

  Used inline in luci.console_view(...) declarations to provide `category` and
  `short_name` for a builder. `console_view` argument can be omitted in this
  case:

      luci.console_view(
          name = 'CI builders',
          ...
          entries = [
              luci.console_view_entry(
                  builder = 'Windows Builder',
                  short_name = 'win',
                  category = 'ci',
              ),
              ...
          ],
      )

  Can also be used to declare that a builder belongs to a console outside of
  the console declaration. In particular useful in functions. For example:

      luci.console_view(name = 'CI builders')

      def ci_builder(name, ...):
        luci.builder(name = name, ...)
        luci.console_view_entry(console_view = 'CI builders', builder = name)

  Args:
    builder: a builder to add, see luci.builder(...). Can be omitted for **extra
        deprecated** case of Buildbot-only views. `buildbot` field must be set
        in this case.
    short_name: a string with the 1-3 character abbreviation of the builder.
    category: a string of the form `term1|term2|...` that describes the
        hierarchy of the builder on the header of the console. Neighboring
        builders with common ancestors will be have their headers merged.
    console_view: a console view to add the builder to. Can be omitted if
        `console_view_entry` is used inline inside some luci.console_view(...)
        declaration.
    buildbot: a reference to an equivalent Buildbot builder, given as
        `<master>/<builder>` string. **Deprecated**. Exists only to aid in the
        migration off Buildbot.
  """
  if builder != None:
    builder = keys.builder_ref(builder, attr='builder')
  if console_view != None:
    console_view = keys.console_view(console_view)
  buildbot = validate.string('buildbot', buildbot, required=False)

  if builder == None:
    if buildbot == None:
      fail('either "builder" or "buildbot" are required')
    key_name = buildbot
  else:
    key_name = builder.id

  # Note: name of this node is important only for error messages. It isn't
  # showing up in any generated files and by construction it can't accidentally
  # collide with some other name.
  key = keys.unique(kinds.CONSOLE_VIEW_ENTRY, key_name)
  graph.add_node(key, props = {
      'short_name': validate.string('short_name', short_name, required=False),
      'category': validate.string('category', category, required=False),
      'buildbot': buildbot,
  })

  if builder != None:
    graph.add_edge(parent=key, child=builder)
  if console_view != None:
    graph.add_edge(parent=console_view, child=key)

  # This is used to detect console_view_entry nodes that aren't connected to any
  # console_view. Such orphan nodes aren't allowed.
  graph.add_edge(parent=keys.milo(), child=key)

  return graph.keyset(key)
