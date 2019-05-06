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

"""CQ related supporting structs and functions."""

load('@stdlib//internal/validate.star', 'validate')
load('@proto//luci/cq/project_config.proto', cq_pb='cq.config')


# A struct returned by cq.refset(...).
#
# See cq.refset(...) function for all details.
#
# Fields (all private to discourage snooping):
#   __repo: original 'repo' string as is.
#   __refs: a list of regexps for refs in the repo, as is.
#   __kind: currently always 'gob'.
#   __repo_key: a tuple with the key to use to represent the repo in dicts.
#   __gob_host: name of a gob host (e.g. 'chromium').
#   __gob_proj: name of a project on this host (e.g. 'infra/luci-py').
_refset_ctor = __native__.genstruct('cq.refset')


# A struct returned by cq.retry_config(...).
_retry_config_ctor = __native__.genstruct('cq.retry_config')


def _refset(repo=None, *, refs=None):
  """Defines a repository and a subset of its refs.

  Used in `watch` field of luci.cq_group(...) to specify what refs the CQ should
  be monitoring.

  *** note
  **Note:** Gerrit ACLs must be configured such that the CQ has read access to
  these refs, otherwise users will be waiting for the CQ to act on their CLs
  forever.
  ***

  Args:
    repo: URL of a git repository to watch, starting with `https://`. Only
        repositories hosted on `*.googlesource.com` are supported currently.
        Required.
    refs: a list of regular expressions that define the set of refs to watch for
        CLs, e.g. `refs/heads/.+`. If not set, defaults to `refs/heads/master`.

  Returns:
    An opaque struct to be passed to `watch` field of luci.cq_group(...).
  """
  repo = validate.repo_url('repo', repo)

  # Deconstruct GoB URL into a (host, repo) tuple. Support only public GoB URLs.

  host, _, proj = repo[len('https://'):].partition('/')

  if not host.endswith('.googlesource.com'):
    fail('bad "repo": only *.googlesource.com repos are supported currently')
  gob = host[:-len('.googlesource.com')]
  if gob.endswith('-review'):
    gob = gob[:-len('-review')]
  if not gob:
    fail('bad "repo": not a valid repository URL')

  if proj.startswith('a/'):
    proj = proj[len('a/'):]
  if proj.endswith('.git'):
    proj = proj[:-len('.git')]
  if not proj:
    fail('bad "repo": not a valid repository URL')

  refs = validate.list('refs', refs)
  for r in refs:
    validate.string('refs', r)

  return _refset_ctor(
      __repo = repo,
      __refs = refs or ['refs/heads/master'],
      __kind = 'gob',
      __repo_key = ('gob', gob, proj),
      __gob_host = gob,
      __gob_proj = proj,
  )


def _validate_refset(attr, val, default=None, required=True):
  """Validates that `val` was constructed via cq.refset(...)."""
  return validate.struct(attr, val, _refset_ctor, default=default, required=required)


def _retry_config(
      *,
      single_quota=None,
      global_quota=None,
      failure_weight=None,
      transient_failure_weight=None,
      timeout_weight=None
  ):
  """Collection of parameters for deciding whether to retry a single build.

  All parameters are integers, with default value of 0. The returned struct
  can be passed as `retry_config` field to luci.cq_group(...).

  Some commonly used presents are available as `cq.RETRY_*` constants. See
  [CQ](#cq_doc) for more info.

  Args:
    single_quota: retry quota for a single tryjob.
    global_quota: retry quota for all tryjobs in a CL.
    failure_weight: the weight assigned to each tryjob failure.
    transient_failure_weight: the weight assigned to each transient (aka
        "infra") failure.
    timeout_weight: weight assigned to tryjob timeouts.

  Returns:
    cq.retry_config struct.
  """
  val_int = lambda attr, val: validate.int(attr, val, min=0, default=0, required=False)
  return _retry_config_ctor(
      single_quota = val_int('single_quota', single_quota),
      global_quota = val_int('global_quota', global_quota),
      failure_weight = val_int('failure_weight', failure_weight),
      transient_failure_weight = val_int('transient_failure_weight', transient_failure_weight),
      timeout_weight = val_int('timeout_weight', timeout_weight),
  )


def _validate_retry_config(attr, val, default=None, required=True):
  """Validates that `val` was constructed via cq.retry_config(...)."""
  return validate.struct(attr, val, _retry_config_ctor, default=default, required=required)


# CQ module exposes structs and enums useful when defining luci.cq_group(...)
# entities.
#
# `cq.ACTION_*` constants define possible values for
# `allow_owner_if_submittable` field of luci.cq_group(...):
#
#   * **cq.ACTION_NONE**: don't grant additional rights to CL owners beyond
#     permissions granted based on owner's roles `CQ_COMMITTER` or
#     `CQ_DRY_RUNNER` (if any).
#   * **cq.ACTION_DRY_RUN** grants the CL owner dry run permission, even if they
#     don't have `CQ_DRY_RUNNER` role.
#   * **cq.ACTION_COMMIT** grants the CL owner commit and dry run permissions,
#     even if they don't have `CQ_COMMITTER` role.
#
# `cq.RETRY_*` constants define some commonly used values for `retry_config`
# field of luci.cq_group(...):
#
#   * **cq.RETRY_NONE**: never retry any failures.
#   * **cq.RETRY_TRANSIENT_FAILURES**: retry only transient (aka "infra")
#     failures. Do at most 2 retries across all builders. Each individual
#     builder is retried at most once. This is the default.
#   * **cq.RETRY_ALL_FAILURES**: retry all failures: transient (aka "infra")
#     failures, real test breakages, and timeouts due to lack of available bots.
#     For non-timeout failures, do at most 2 retries across all builders. Each
#     individual builder is retried at most once. Timeout failures are
#     considered "twice as heavy" as non-timeout failures (e.g. one retried
#     timeout failure immediately exhausts all retry quota for the CQ attempt).
#     This is to avoid adding more requests to an already overloaded system.
cq = struct(
    refset = _refset,
    retry_config = _retry_config,

    ACTION_NONE = cq_pb.Verifiers.GerritCQAbility.UNSET,
    ACTION_DRY_RUN = cq_pb.Verifiers.GerritCQAbility.DRY_RUN,
    ACTION_COMMIT = cq_pb.Verifiers.GerritCQAbility.COMMIT,

    RETRY_NONE = _retry_config(),
    RETRY_TRANSIENT_FAILURES = _retry_config(
        single_quota = 1,
        global_quota = 2,
        failure_weight = 100,  # +inf
        transient_failure_weight = 1,
        timeout_weight = 100,  # +inf
    ),
    RETRY_ALL_FAILURES = _retry_config(
        single_quota = 1,
        global_quota = 2,
        failure_weight = 1,
        transient_failure_weight = 1,
        timeout_weight = 2,
    ),
)

cqimpl = struct(
    validate_refset = _validate_refset,
    validate_retry_config = _validate_retry_config,
)
