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
load('@stdlib//internal/io.star', 'io')
load('@stdlib//internal/lucicfg.star', 'lucicfg')
load('@stdlib//internal/validate.star', 'validate')

load('@stdlib//internal/luci/common.star', 'keys', 'kinds', 'view')
load('@stdlib//internal/luci/rules/console_view_entry.star', 'console_view_entry')

load('@proto//luci/milo/project_config.proto', milo_pb='milo')


# TODO(vadimsh): Document how builders should be configured to be eligible for
# inclusion into a console.


def _console_view(
      ctx,
      *,

      name=None,
      title=None,
      repo=None,
      refs=None,
      exclude_ref=None,
      include_experimental_builds=None,
      header=None,
      favicon=None,
      entries=None,
      default_commit_limit=None,
      default_expand=None
  ):
  """A Milo UI view that displays a table-like console where columns are
  builders and rows are git commits on which builders are triggered.

  A console is associated with a single git repository it uses as a source of
  commits to display as rows. The watched ref set is defined via `refs` and
  optional `exclude_ref` fields. If `refs` are empty, the console defaults to
  watching `refs/heads/master`.

  `exclude_ref` is useful when watching for commits that landed specifically
  on a branch. For example, the config below allows to track commits from all
  release branches, but ignore the commits from the master branch, from which
  these release branches are branched off:

      luci.console_view(
          ...
          refs = ['refs/branch-heads/\d+\.\d+'],
          exclude_ref = 'refs/heads/master',
          ...
      )

  For best results, ensure commits on each watched ref have **committer**
  timestamps monotonically non-decreasing. Gerrit will take care of this if you
  require each commit to go through Gerrit by prohibiting "git push" on these
  refs.

  #### Adding builders

  Builders that belong to the console can be specified either right here:

      luci.console_view(
          name = 'CI builders',
          ...
          entries = [
              luci.console_view_entry(
                  builder = 'Windows Builder',
                  short_name = 'win',
                  category = 'ci',
              ),
              # Can also pass a dict, this is equivalent to passing
              # luci.console_view_entry(**dict).
              {
                  'builder': 'Linux Builder',
                  'short_name': 'lnx',
                  'category': 'ci',
              },
              ...
          ],
      )

  Or separately one by one via luci.console_view_entry(...) declarations:

      luci.console_view(name = 'CI builders')
      luci.console_view_entry(
          builder = 'Windows Builder',
          console_view = 'CI builders',
          short_name = 'win',
          category = 'ci',
      )

  #### Console headers

  Consoles can have headers which are collections of links, oncall rotation
  information, and console summaries that are displayed at the top of a console,
  below the tree status information. Links and oncall information is always laid
  out to the left, while console groups are laid out to the right. Each oncall
  and links group take up a row.

  Header definitions are based on `Header` message in Milo's [project.proto].
  There are two way to supply this message via `header` field:

    * Pass an appropriately structured dict. Useful for defining small headers
      inline:

          luci.console_view(
              ...
              header = {
                  'links': [
                      {'name': '...', 'links': [...]},
                      ...
                  ],
              },
              ...
          )

    * Pass a string. It is treated as a path to a file with serialized
      `Header` message. Depending on its extension, it is loaded ether as
      JSONPB-encoded message (`*.json` and `*.jsonpb` paths), or as
      TextPB-encoded message (everything else):

          luci.console_view(
              ...
              header = '//consoles/main_header.textpb',
              ...
          )

  [project.proto]: https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/master/milo/api/config/project.proto

  Args:
    name: a name of this console, will show up in URLs. Note that names of
        luci.console_view(...) and luci.list_view(...) are in the same namespace
        i.e. defining a console view with the same name as some list view (and
        vice versa) causes an error. Required.
    title: a title of this console, will show up in UI. Defaults to `name`.
    repo: URL of a git repository whose commits are displayed as rows in the
        console. Must start with `https://`. Required.
    refs: a list of regular expressions that define the set of refs to pull
        commits from when displaying the console, e.g. `refs/heads/[^/]+` or
        `refs/branch-heads/\d+\.\d+`. The regular expression should have a
        literal prefix with at least two slashes present, e.g.
        `refs/release-\d+/foobar` is *not allowed*, because the literal prefix
        `refs/release-` contains only one slash. The regexp should not start
        with `^` or end with `$` as they will be added automatically.  If empty,
        defaults to `['refs/heads/master']`.
    exclude_ref: a single ref, commits from which are ignored even when they are
        reachable from refs specified via `refs` and `refs_regexps`. Note that
        force pushes to this ref are not supported. Milo uses caching assuming
        set of commits reachable from this ref may only grow, never lose some
        commits.
    header: either a string with a path to the file with the header definition
        (see io.read_file(...) for the acceptable path format), or a dict with
        the header definition.
    include_experimental_builds: if True, this console will not filter out
        builds marked as Experimental. By default consoles only show production
        builds.
    favicon: optional https URL to the favicon for this console, must be hosted
        on `storage.googleapis.com`. Defaults to `favicon` in luci.milo(...).
    entries: a list of luci.console_view_entry(...) entities specifying builders
        to show on the console.
    default_commit_list: if set, will change the default number of commits to
        query on a single page.
    default_expand: if set, will default the console page to expanded view.
  """
  refs = validate.list('refs', refs) or ['refs/heads/master']
  for r in refs:
    validate.string('refs', r)

  if header != None:
    if type(header) == 'dict':
      header = milo_pb.Header(**header)
    elif type(header) == 'string':
      header = io.read_proto(milo_pb.Header, header)
    else:
      fail('bad "header": got %s, want string or dict' % type(header))

  return view.add_view(
      key = keys.console_view(name),
      entry_kind = kinds.CONSOLE_VIEW_ENTRY,
      entry_ctor = console_view_entry,
      entries = entries,
      props = {
          'name': name,
          'title': validate.string('title', title, default=name, required=False),
          'repo': validate.string('repo', repo, regexp=r'https://.+'),
          'refs': refs,
          'exclude_ref': validate.string('exclude_ref', exclude_ref, required=False),
          'header': header,
          'include_experimental_builds': validate.bool('include_experimental_builds', include_experimental_builds, required=False),
          'favicon': validate.string('favicon', favicon, regexp=r'https://storage\.googleapis\.com/.+', required=False),
          'default_commit_limit': validate.int('default_commit_limit', default_commit_limit, 0, 1000, 0, required=False),
          'default_expand': validate.bool('default_expand', default_expand, required=False),
      },
  )


console_view = lucicfg.rule(impl = _console_view)
