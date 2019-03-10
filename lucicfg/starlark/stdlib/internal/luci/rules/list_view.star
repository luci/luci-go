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

load('@stdlib//internal/luci/common.star', 'keys', 'kinds', 'view')
load('@stdlib//internal/luci/rules/list_view_entry.star', 'list_view_entry')


def _list_view(
      ctx,
      *,
      name=None,
      title=None,
      favicon=None,
      entries=None
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
    name: a name of this view, will show up in URLs. Note that names of
        luci.list_view(...) and luci.console_view(...) are in the same namespace
        i.e. defining a list view with the same name as some console view (and
        vice versa) causes an error. Required.
    title: a title of this view, will show up in UI. Defaults to `name`.
    favicon: optional https URL to the favicon for this view, must be hosted on
        `storage.googleapis.com`. Defaults to `favicon` in luci.milo(...).
    entries: a list of builders or luci.list_view_entry(...) entities to include
        into this view.
  """
  return view.add_view(
      key = keys.list_view(name),
      entry_kind = kinds.LIST_VIEW_ENTRY,
      entry_ctor = list_view_entry,
      entries = entries,
      props = {
          'name': name,
          'title': validate.string('title', title, default=name, required=False),
          'favicon': validate.string('favicon', favicon, regexp=r'https://storage\.googleapis\.com/.+', required=False),
      },
  )


list_view = lucicfg.rule(impl = _list_view)
