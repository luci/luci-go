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
load('@stdlib//internal/luci/rules/list_view_entry.star', 'list_view_entry')


def list_view(
      name=None,
      title=None,
      favicon=None,
      entries=None,
  ):
  """A Milo UI view that displays a list of builders.

  Builders that belong to this view can be specified either right here:

      luci.list_view(
          name = 'Try builders',
          entries = [
              'win',
              'linux',
              luci.list_view_entry('osx'),
          ],
      )

  Or separately one by one via luci.list_view_entry(...) declarations:

      luci.list_view(name = 'Try builders')
      luci.list_view_entry(
          builder = 'win',
          list_view = 'Try builders',
      )
      luci.list_view_entry(
          builder = 'linux',
          list_view = 'Try builders',
      )

  Note that declaring Buildbot builders (which is deprecated) requires the use
  of luci.list_view_entry(...). It's the only way to provide a reference to a
  Buildbot builder (see `buildbot` field).

  Args:
    name: a name of this view, will show up in URLs. Required.
    title: a title of this view, will show up in UI. Defaults to `name`.
    favicon: optional https URL to the favicon for this view, must be hosted on
        `storage.googleapis.com`. Defaults to `favicon` in luci.milo(...).
    entries: a list of builders or luci.list_view_entry(...) entities to include
        into this view.
  """
  key = keys.list_view(name)
  graph.add_node(key, props = {
      'name': key.id,
      'title': validate.string('title', title, default=key.id, required=False),
      'favicon': validate.string('favicon', favicon, regexp=r'https://storage\.googleapis\.com/.+', required=False),
  })
  graph.add_edge(
      parent = keys.project(),
      child = key,
  )

  # 'entry' is either a list_view_entry keyset or a builder_ref (perhaps given
  # as a string). If it is a builder_ref, we add a list_view_entry for it
  # automatically, so in the end all children have the same kind. That makes
  # the generator simpler.
  for entry in validate.list('entries', entries):
    if type(entry) == 'string' or (graph.is_keyset(entry) and entry.has(kinds.BUILDER_REF)):
      entry = list_view_entry(builder = entry)
    graph.add_edge(
        parent = key,
        child = entry.get(kinds.LIST_VIEW_ENTRY),
    )

  return graph.keyset(key)
