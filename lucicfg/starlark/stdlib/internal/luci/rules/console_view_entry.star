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

load('@stdlib//internal/lucicfg.star', 'lucicfg')
load('@stdlib//internal/validate.star', 'validate')

load('@stdlib//internal/luci/common.star', 'keys', 'kinds', 'view')


def _console_view_entry(
      ctx,
      builder=None,
      *,
      short_name=None,
      category=None,
      console_view=None,
      buildbot=None
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
    builder: a builder to add, see luci.builder(...). Can also be a reference
        to a builder defined in another project. See [Referring to builders in
        other projects](#external_builders) for more details. Can be omitted
        for **extra deprecated** case of Buildbot-only views. `buildbot` field
        must be set in this case.
    short_name: a shorter name of the builder. The recommendation is to keep
        this name as short as reasonable, as longer names take up more
        horizontal space.
    category: a string of the form `term1|term2|...` that describes the
        hierarchy of the builder columns. Neighboring builders with common
        ancestors will have their column headers merged. In expanded view, each
        leaf category or builder under a non-leaf category will have it's own
        column. The recommendation for maximum densification is not to mix
        subcategories and builders for children of each category.
    console_view: a console view to add the builder to. Can be omitted if
        `console_view_entry` is used inline inside some luci.console_view(...)
        declaration.
    buildbot: a reference to an equivalent Buildbot builder, given as
        `<master>/<builder>` string. **Deprecated**. Exists only to aid in the
        migration off Buildbot.
  """
  return view.add_entry(
      kind = kinds.CONSOLE_VIEW_ENTRY,
      view = keys.console_view(console_view) if console_view else None,
      builder = builder,
      buildbot = buildbot,
      props = {
          'short_name': validate.string('short_name', short_name, required=False),
          'category': validate.string('category', category, required=False),
      },
  )


console_view_entry = lucicfg.rule(impl = _console_view_entry)
