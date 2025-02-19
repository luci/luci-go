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

"""Defines luci.gitiles_poller(...) rule."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "keys", "triggerer")

def _gitiles_poller(
        ctx,  # @unused
        *,
        name = None,
        bucket = None,
        repo = None,
        refs = None,
        path_regexps = None,
        path_regexps_exclude = None,
        schedule = None,
        triggers = None):
    r"""Defines a gitiles poller which can trigger builders on git commits.

    It periodically examines the state of watched refs in the git repository. On
    each iteration it triggers builders if either:

      * A watched ref's tip has changed since the last iteration (e.g. a new
        commit landed on a ref). Each new detected commit results in a separate
        triggering request, so if for example 10 new commits landed on a ref
        since the last poll, 10 new triggering requests will be submitted to the
        builders triggered by this poller. How they are converted to actual
        builds depends on `triggering_policy` of a builder. For example, some
        builders may want to have one build per commit, others don't care and
        just want to test the latest commit. See luci.builder(...) and
        scheduler.policy(...) for more details.

        *** note
        **Caveat**: When a large number of commits are pushed on the ref between
        iterations of the poller, only the most recent 50 commits will result in
        triggering requests. Everything older is silently ignored. This is a
        safeguard against mistaken or deliberate but unusual git push actions,
        which typically don't have the intent of triggering a build for each
        such commit.
        ***

      * A ref belonging to the watched set has just been created. This produces
        a single triggering request for the commit at the ref's tip.
        This also applies right after a configuration change which instructs the
        scheduler to watch a new ref, unless the ref is a tag; only tags that
        appear *after* a change to the poller's refs will be considered new.
        Newly matched but pre-existing tags will not produce triggering
        requests, since there may be many of them and it's rare that a job needs
        to be triggered on all historical tags.

    Commits that trigger builders can also optionally be filtered by file paths
    they touch. These conditions are specified via `path_regexps` and
    `path_regexps_exclude` fields, each is a list of regular expressions against
    Unix file paths relative to the repository root. A file is considered
    "touched" if it is either added, modified, removed, moved (both old and new
    paths are considered "touched"), or its metadata has changed (e.g.
    `chmod +x`).

    A triggering request is emitted for a commit if only if at least one touched
    file is *not* matched by any `path_regexps_exclude` *and* simultaneously
    matched by some `path_regexps`, subject to following caveats:

      * `path_regexps = [".+"]` will *not* match commits which modify no files
        (aka empty commits) and as such this situation differs from the default
        case of not specifying any `path_regexps`.
      * As mentioned above, if a ref fast-forwards >=50 commits, only the last
        50 commits are checked. The rest are ignored.

    A luci.gitiles_poller(...) with some particular name can be redeclared many
    times as long as all fields in all declaration are identical. This is
    helpful when luci.gitiles_poller(...) is used inside a helper function that
    at once declares a builder and a poller that triggers this builder.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      name: name of the poller, to refer to it from other rules. Required.
      bucket: a bucket the poller is in, see luci.bucket(...) rule. Required.
      repo: URL of a git repository to poll, starting with `https://`. Required.
      refs: a list of regular expressions that define the watched set of refs,
        e.g. `refs/heads/[^/]+` or `refs/branch-heads/\d+\.\d+`. The regular
        expression should have a literal prefix with at least two slashes
        present, e.g. `refs/release-\d+/foobar` is *not allowed*, because the
        literal prefix `refs/release-` contains only one slash. The regexp
        should not start with `^` or end with `$` as they will be added
        automatically. Each supplied regexp must match at least one ref in the
        gitiles output, e.g. specifying `refs/tags/v.+` for a repo that doesn't
        have tags starting with `v` causes a runtime error. If empty, defaults
        to `['refs/heads/main']`.
      path_regexps: a list of regexps that define a set of files to watch for
        changes. `^` and `$` are implied and should not be specified manually.
        See the explanation above for all details.
      path_regexps_exclude: a list of regexps that define a set of files to
        *ignore* when watching for changes. `^` and `$` are implied and should
        not be specified manually. See the explanation above for all details.
      schedule: string with a schedule that describes when to run one iteration
        of the poller. See [Defining cron schedules](#schedules-doc) for the
        expected format of this field. Note that it is rare to use custom
        schedules for pollers. By default, the poller will run each 30 sec.
      triggers: builders to trigger whenever the poller detects a new git commit
        on any ref in the watched ref set.
    """
    name = validate.string("name", name)
    bucket_key = keys.bucket(bucket)

    refs = validate.list("refs", refs) or ["refs/heads/main"]
    for r in refs:
        validate.string("refs", r)

    path_regexps = validate.list("path_regexps", path_regexps)
    for p in path_regexps:
        validate.string("path_regexps", p)

    path_regexps_exclude = validate.list("path_regexps_exclude", path_regexps_exclude)
    for p in path_regexps_exclude:
        validate.string("path_regexps_exclude", p)

    # Node that carries the full definition of the poller.
    poller_key = keys.gitiles_poller(bucket_key.id, name)
    graph.add_node(poller_key, idempotent = True, props = {
        "name": name,
        "bucket": bucket_key.id,
        "realm": bucket_key.id,
        "repo": validate.repo_url("repo", repo),
        "refs": refs,
        "path_regexps": path_regexps,
        "path_regexps_exclude": path_regexps_exclude,
        "schedule": validate.string("schedule", schedule, required = False),
    })
    graph.add_edge(bucket_key, poller_key)

    # Setup nodes that indicate this poller can be referenced in 'triggered_by'
    # relations (either via its bucket-scoped name or via its global name).
    triggerer_key = triggerer.add(poller_key, idempotent = True)

    # Link to builders triggered by this builder.
    for t in validate.list("triggers", triggers):
        graph.add_edge(
            parent = triggerer_key,
            child = keys.builder_ref(t),
            title = "triggers",
        )

    return graph.keyset(poller_key, triggerer_key)

gitiles_poller = lucicfg.rule(impl = _gitiles_poller)
