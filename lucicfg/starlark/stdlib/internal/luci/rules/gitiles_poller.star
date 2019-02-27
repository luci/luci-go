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

load('@stdlib//internal/luci/common.star', 'keys', 'triggerer')


def gitiles_poller(
      *,
      name=None,
      bucket=None,
      repo=None,
      refs=None,
      schedule=None,
      triggers=None
  ):
  """Defines a gitiles poller which can trigger builders on git commits.

  It periodically examines the state of watched refs in the git repository. On
  each iteration it triggers builders if either:

    * A watched ref's tip has changed since the last iteration (e.g. a new
      commit landed on a ref). Each new detected commit results in a separate
      triggering request, so if for example 10 new commits landed on a ref since
      the last poll, 10 new triggering requests will be submitted to the
      builders triggered by this poller. How they are converted to actual builds
      depends on `triggering_policy` of a builder. For example, some builders
      may want to have one build per commit, others don't care and just want to
      test the latest commit. See luci.builder(...) and scheduler.policy(...)
      for more details.

      *** note
      **Caveat**: When a large number of commits are pushed on the ref between
      iterations of the poller, only the most recent 50 commits will result in
      triggering requests. Everything older is silently ignored. This is a
      safeguard against mistaken or deliberate but unusual git push actions,
      which typically don't have intent of triggering a build for each such
      commit.
      ***

    * A ref belonging to the watched set has just been created. This produces
      a single triggering request.

  Args:
    name: name of the poller, to refer to it from other rules. Required.
    bucket: a bucket the poller is in, see luci.bucket(...) rule. Required.
    repo: URL of a git repository to poll, starting with `https://`. Required.
    refs: a list of regular expressions that define the watched set of refs,
        e.g. `refs/heads/[^/]+` or `refs/branch-heads/\d+\.\d+`. The regular
        expression should have a literal prefix with at least two slashes
        present, e.g. `refs/release-\d+/foobar` is *not allowed*, because the
        literal prefix `refs/release-` contains only one slash. The regexp
        should not start with `^` or end with `$` as they will be added
        automatically. If empty, defaults to `['refs/heads/master']`.
    schedule: string with a schedule that describes when to run one iteration
        of the poller. See [Defining cron schedules](#schedules_doc) for the
        expected format of this field. Note that it is rare to use custom
        schedules for pollers. By default, the poller will run each 30 sec.
    triggers: builders to trigger whenever the poller detects a new git
        commit on any ref in the watched ref set.
  """
  name = validate.string('name', name)
  bucket_key = keys.bucket(bucket)

  refs = validate.list('refs', refs) or ['refs/heads/master']
  for r in refs:
    validate.string('refs', r)

  # Node that carries the full definition of the poller.
  poller_key = keys.gitiles_poller(bucket_key.id, name)
  graph.add_node(poller_key, props = {
      'name': name,
      'bucket': bucket_key.id,
      'repo': validate.string('repo', repo, regexp=r'https://.+'),
      'refs': refs,
      'schedule': validate.string('schedule', schedule, required=False),
  })
  graph.add_edge(bucket_key, poller_key)

  # Setup nodes that indicate this poller can be referenced in 'triggered_by'
  # relations (either via its bucket-scoped name or via its global name).
  triggerer_key = triggerer.add(poller_key)

  # Link to builders triggered by this builder.
  for t in validate.list('triggers', triggers):
    graph.add_edge(
        parent = triggerer_key,
        child = keys.builder_ref(t),
        title = 'triggers',
    )

  return graph.keyset(poller_key, triggerer_key)
