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
load('@stdlib//internal/luci/common.star', 'keys', 'triggerer')
load('@stdlib//internal/luci/lib/validate.star', 'validate')


def gitiles_poller(
      name=None,
      bucket=None,
      repo=None,
      refs=None,
      refs_regexps=None,
      schedule=None,
      triggers=None,
  ):
  """Defines a gitiles poller which can trigger builders on git commits.

  It watches a set of git refs and triggers builders if either:

    * A watched ref's tip has changed (e.g. a new commit landed on a ref).
    * A ref belonging to the watched set has just been created.

  The watched ref set is defined via `refs` and `refs_regexps` fields. One is
  just a simple enumeration of refs, and another allows to use regular
  expressions to define what refs belong to the watched set. Both fields can
  be used at the same time. If neither is set, the gitiles_poller defaults to
  watching `refs/heads/master`.

  Args:
    name: name of the poller, to refer to it from other rules. Required.
    bucket: name of the bucket the poller belongs to. Required.
    repo: URL of a git repository to poll, starting with `https://`. Required.
    refs: a list of fully qualified refs to watch, e.g. `refs/heads/master` or
        `refs/tags/v1.2.3`.
    refs_regexps: a list of regular expressions that define the watched set of
        refs, e.g. `refs/heads/[^/]+` or `refs/branch-heads/\d+\.\d+`. The
        regular expression should have a literal prefix with at least two
        slashes present, e.g. `refs/release-\d+/foobar` is *not allowed*,
        because the literal prefix `refs/release-` contains only one slash. The
        regexp should not start with `^` or end with `$` as they will be added
        automatically.
    schedule: string with a schedule that describes when to run one iteration
        of the poller. See [Defining cron schedules](#schedules_doc) for the
        expected format of this field. Note that it is rare to use custom
        schedules for pollers. By default, the poller will run each 30 sec.
    triggers: names of builders to trigger whenever the poller detects a new git
        commit on any ref in the watched ref set.
  """
  name = validate.string('name', name)
  bucket = validate.string('bucket', bucket)

  refs = validate.list('refs', refs)
  for r in refs:
    validate.string('refs', r)

  refs_regexps = validate.list('refs_regexps', refs_regexps)
  for r in refs_regexps:
    validate.string('refs_regexps', r)

  if not refs and not refs_regexps:
    refs = ['refs/heads/master']

  # Node that carries the full definition of the poller.
  poller_key = keys.gitiles_poller(bucket, name)
  graph.add_node(poller_key, props = {
      'name': name,
      'bucket': bucket,
      'repo': validate.string('repo', repo, regexp=r'https://.+'),
      'refs': refs,
      'refs_regexps': refs_regexps,
      'schedule': validate.string('schedule', schedule, required=False),
  })
  graph.add_edge(keys.bucket(bucket), poller_key)

  # Setup nodes that indicate this poller can be referenced in 'triggered_by'
  # relations (either via its bucket-scoped name or via its global name).
  triggerer_key = triggerer.add(poller_key)

  # Link to builders triggered by this builder.
  for t in validate.list('triggers', triggers):
    graph.add_edge(
        parent = triggerer_key,
        child = keys.builder_ref(validate.string('triggers', t)),
        title = 'triggers',
    )
