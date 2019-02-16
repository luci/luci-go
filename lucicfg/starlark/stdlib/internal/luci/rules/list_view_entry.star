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


def list_view_entry(builder=None, list_view=None, buildbot=None):
  """A builder entry in some luci.list_view(...).

  Can be used to declare that a builder belongs to a list view outside of
  the list view declaration. In particular useful in functions. For example:

      luci.list_view(name = 'Try builders')

      def try_builder(name, ...):
        luci.builder(name = name, ...)
        luci.list_view_entry(list_view = 'Try builders', builder = name)

  Can also be used inline in luci.list_view(...) declarations, for consistency
  with corresponding luci.console_view_entry(...) usage. `list_view` argument
  can be omitted in this case:

      luci.list_view(
          name = 'Try builders',
          entries = [
              luci.list_view_entry(builder = 'Win'),
              ...
          ],
      )

  Args:
    builder: a builder to add, see luci.builder(...). Can be omitted for **extra
        deprecated** case of Buildbot-only views. `buildbot` field must be set
        in this case.
    list_view: a list view to add the builder to. Can be omitted if
        `list_view_entry` is used inline inside some luci.list_view(...)
        declaration.
    buildbot: a reference to an equivalent Buildbot builder, given as
        `<master>/<builder>` string. **Deprecated**. Exists only to aid in the
        migration off Buildbot.
  """
  if builder != None:
    builder = keys.builder_ref(builder, attr='builder')
  if list_view != None:
    list_view = keys.list_view(list_view)
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
  key = keys.unique(kinds.LIST_VIEW_ENTRY, key_name)
  graph.add_node(key, props = {
      'buildbot': buildbot,
  })

  if builder != None:
    graph.add_edge(parent=key, child=builder)
  if list_view != None:
    graph.add_edge(parent=list_view, child=key)

  # This is used to detect list_view_entry nodes that aren't connected to any
  # list_view. Such orphan nodes aren't allowed.
  graph.add_edge(parent=keys.milo(), child=key)

  return graph.keyset(key)
