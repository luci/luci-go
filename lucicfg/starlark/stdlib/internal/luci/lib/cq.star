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

load("@stdlib//internal/luci/proto.star", "cq_pb")
load("@stdlib//internal/validate.star", "validate")

# A struct returned by cq.refset(...).
#
# See cq.refset(...) function for all details.
#
# Fields (all private to discourage snooping):
#   __repo: original 'repo' string as is.
#   __refs: a list of regexps for refs in the repo, as is.
#   __refs_exclude: a list of regexps for refs to exclude, as is.
#   __kind: currently always 'gob'.
#   __repo_key: a tuple with the key to use to represent the repo in dicts.
#   __gob_host: name of a gob host (e.g. 'chromium').
#   __gob_proj: name of a project on this host (e.g. 'infra/luci-py').
_refset_ctor = __native__.genstruct("cq.refset")

# A struct returned by cq.retry_config(...).
_retry_config_ctor = __native__.genstruct("cq.retry_config")

# A struct returned by cq.run_mode(...).
_run_mode_ctor = __native__.genstruct("cq.run_mode")

# A struct returned by cq.location_filter(...).
_location_filter_ctor = __native__.genstruct("cq.location_filter")

# A struct returned by cq.quota_policy(...).
_quota_policy_ctor = __native__.genstruct("cq.quota_policy")

# A struct returned by cq.run_limits(...).
_run_limits_ctor = __native__.genstruct("cq.run_limits")

# A struct returned by cq.tryjob_limits(...).
_tryjob_limits_ctor = __native__.genstruct("cq.tryjob_limits")

def _refset(repo = None, *, refs = None, refs_exclude = None):
    """Defines a repository and a subset of its refs.

    Used in `watch` field of luci.cq_group(...) to specify what refs the CQ
    should be monitoring.

    *** note
    **Note:** Gerrit ACLs must be configured such that the CQ has read access to
    these refs, otherwise users will be waiting for the CQ to act on their CLs
    forever.
    ***

    Args:
      repo: URL of a git repository to watch, starting with `https://`. Only
        repositories hosted on `*.googlesource.com` are supported currently.
        Required.
      refs: a list of regular expressions that define the set of refs to watch
        for CLs, e.g. `refs/heads/.+`. If not set, defaults to
        `refs/heads/main`.
      refs_exclude: a list of regular expressions that define the set of refs
        to exclude from watching. Empty by default.

    Returns:
      An opaque struct to be passed to `watch` field of luci.cq_group(...).
    """
    repo = validate.repo_url("repo", repo)

    # Deconstruct GoB URL into a (host, repo) tuple. Support only public GoB URLs.

    host, _, proj = repo[len("https://"):].partition("/")

    if not host.endswith(".googlesource.com"):
        fail('bad "repo": only *.googlesource.com repos are supported currently')
    gob = host[:-len(".googlesource.com")]
    if gob.endswith("-review"):
        gob = gob[:-len("-review")]
    if not gob:
        fail('bad "repo": not a valid repository URL')

    if proj.startswith("a/"):
        proj = proj[len("a/"):]
    if proj.endswith(".git"):
        proj = proj[:-len(".git")]
    if not proj:
        fail('bad "repo": not a valid repository URL')

    refs = validate.str_list("refs", refs)
    refs_exclude = validate.str_list("refs_exclude", refs_exclude)

    return _refset_ctor(
        __repo = repo,
        __refs = refs or ["refs/heads/main"],
        __refs_exclude = refs_exclude,
        __kind = "gob",
        __repo_key = ("gob", gob, proj),
        __gob_host = gob,
        __gob_proj = proj,
    )

def _validate_refset(attr, val, *, default = None, required = True):
    """Validates that `val` was constructed via cq.refset(...)."""
    return validate.struct(attr, val, _refset_ctor, default = default, required = required)

def _retry_config(
        *,
        single_quota = None,
        global_quota = None,
        failure_weight = None,
        transient_failure_weight = None,
        timeout_weight = None):
    """Collection of parameters for deciding whether to retry a single build.

    All parameters are integers, with default value of 0. The returned struct
    can be passed as `retry_config` field to luci.cq_group(...).

    Some commonly used presents are available as `cq.RETRY_*` constants. See
    [CQ](#cq-doc) for more info.

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
    val_int = lambda attr, val: validate.int(attr, val, min = 0, default = 0, required = False)
    return _retry_config_ctor(
        single_quota = val_int("single_quota", single_quota),
        global_quota = val_int("global_quota", global_quota),
        failure_weight = val_int("failure_weight", failure_weight),
        transient_failure_weight = val_int("transient_failure_weight", transient_failure_weight),
        timeout_weight = val_int("timeout_weight", timeout_weight),
    )

def _validate_retry_config(attr, val, *, default = None, required = True):
    """Validates that `val` was constructed via cq.retry_config(...)."""
    return validate.struct(attr, val, _retry_config_ctor, default = default, required = required)

def _run_mode(
        name = None,
        cq_label_value = None,
        triggering_label = None,
        triggering_value = None):
    """Defines a CQ Run mode and how it can be triggered.

    Args:
      name: name of this mode.
        Must match regex "^[a-zA-Z][a-zA-Z0-9_-]{0,39}$". Required.
      cq_label_value: the value of Commit-Queue label that MUST be set to when
        triggering a CQ Run in this mode. Required.
      triggering_label: the Gerrit label that MUST also be set in order to
        trigger a CQ Run in this mode. Required.
      triggering_value: the value of the `triggering_label` that MUST be set to
        when triggering a CQ Run in this mode. Required.

    Returns:
      cq.run_mode struct.
    """
    if name in ("DRY_RUN", "FULL_RUN"):
        fail('bad "mode_name": "DRY_RUN" and "FULL_RUN" are reserved by CQ')
    return _run_mode_ctor(
        name = validate.string(
            "mode_name",
            name,
            regexp = "^[a-zA-Z][a-zA-Z0-9_-]{0,39}$",
        ),
        cq_label_value = validate.int(
            "cq_label_value",
            cq_label_value,
            min = 1,
            max = 2,
        ),
        triggering_label = validate.string(
            "triggering_label",
            triggering_label,
        ),
        triggering_value = validate.int(
            "triggering_value",
            triggering_value,
            min = 1,
        ),
    )

def _validate_run_mode(attr, val, *, default = None, required = True):
    """Validates that `val` was constructed via cq.run_mode(...)."""
    return validate.struct(attr, val, _run_mode_ctor, default = default, required = required)

def _location_filter(
        gerrit_host_regexp = None,
        gerrit_project_regexp = None,
        path_regexp = None,
        exclude = False):
    """Defines a location filter for the builder location_filters field.

    All regexp fields can be empty, which is treated the same as ".*", i.e. a
    wildcard which should match anything. Patterns are implicitly wrapped with
    "^...$". They are allowed to contain these anchors, but it's redundant.

    Args:
      gerrit_host_regexp: Gerrit host regex. Must be a valid regex.
      gerrit_project_regexp: Gerrit project pattern. Must be a valid regex.
      path_regexp: File path pattern. Must be a valid regex.
      exclude: Whether this is an "exclude" pattern.

    Returns:
      cq.location_filter struct.
    """
    return _location_filter_ctor(
        gerrit_host_regexp = validate.string(
            "gerrit_host_regexp",
            gerrit_host_regexp,
            default = "",
            allow_empty = True,
            required = False,
        ),
        gerrit_project_regexp = validate.string(
            "gerrit_project_regexp",
            gerrit_project_regexp,
            default = "",
            allow_empty = True,
            required = False,
        ),
        path_regexp = validate.string(
            "path_regexp",
            path_regexp,
            default = "",
            allow_empty = True,
            required = False,
        ),
        exclude = validate.bool(
            "exclude",
            exclude,
            default = False,
        ),
    )

def _validate_location_filter(attr, val, *, default = None, required = True):
    """Validates location_filter."""
    return validate.struct(attr, val, _location_filter_ctor, default = default, required = required)

def _validate_quota_policy(attr, val, *, default = None, required = False):
    """Validates that `val` was constructed via cq._quota_policy(...)."""
    return validate.struct(attr, val, _quota_policy_ctor, default = default, required = required)

def _quota_policy(
        name = None,
        users = None,
        groups = None,
        run_limits = None,
        tryjob_limits = None):
    """Construct a quota policy with run and tryjob limits for users and groups.

    At the time of Run creation, CV looks up a quota policy applicable for
    the Run, and blocks processing the Run or the tryjobs, if the remaining
    quota balance is insufficient.

    This constructs and returns a quota policy which specifies run and tryjob
    limits for given users and groups in cq_group(...). Find cq_group(...) to
    find how quota policies are used in cq_group(...).

    Args:
      name: the name of the policy to configure.
        Must be unqiue in the ConfigGroup.
        Must match regex '^[0-9A-Za-z][0-9A-Za-z\\.\\-@_+]{0,511}$'.
        Required.
      users: a list of user identities, which is an email typically, to apply
        the quota policy to.
      groups: a list of chrome infra auth group names to apply the quota policy
        to.
      run_limits: Run limits to apply. See cq.run_limits(...) for more details.
        If omitted, unlimited run quotas are granted to the users and groups.
      tryjob_limits: Tryjob limits to apply. See cq.tryjob_limits(...) for more
        details. If omitted, unlimited tryjob quotas are granted to the users
        and groups.
    """
    name = validate.string(
        "name",
        name,
        required = True,
        regexp = "^[0-9A-Za-z][0-9A-Za-z.\\-@_+]{0,511}$",
    )
    principals = []
    users = validate.list("users", users, required = False)
    groups = validate.list("groups", groups, required = False)
    for i, u in enumerate(users):
        p = "user:%s" % validate.string("users[%d]" % i, u, required = True)
        principals.append(p)
    for i, g in enumerate(groups):
        p = "group:%s" % validate.string("groups[%d]" % i, g, required = True)
        principals.append(p)
    _validate_run_limits("run_limits", run_limits, required = False)
    _validate_tryjob_limits("tryjob_limits", tryjob_limits, required = False)

    return _quota_policy_ctor(
        name = name,
        principals = principals,
        run_limits = run_limits,
        tryjob_limits = tryjob_limits,
    )

def _validate_run_limits(attr, val, *, default = None, required = False):
    """Validates that `val` was constructed via cq.run_limits(...)."""
    return validate.struct(attr, val, _run_limits_ctor, default = default, required = required)

def _run_limits(max_active = None):
    """Constructs Run limits.

    All limit values must be > 0, or None if no limit.

    Args:
      max_active: Max number of ongoing Runs that there can be at any moment.
    """
    return _run_limits_ctor(
        max_active = validate.int(
            "max_active",
            max_active,
            min = 1,
            default = None,
            required = False,
        ),
    )

def _validate_tryjob_limits(attr, val, *, default = None, required = False):
    """Validates that `val` was constructed via cq.tryjob_limits(...)."""
    return validate.struct(attr, val, _tryjob_limits_ctor, default = default, required = required)

def _tryjob_limits(max_active = None):
    """Constructs Tryjob limits.

    All limit values must be > 0, or None if no limit.

    Args:
      max_active: Max number of ongoing Tryjobs that there can be at any moment.
    """
    return _tryjob_limits_ctor(
        max_active = validate.int(
            "max_active",
            max_active,
            min = 1,
            default = None,
            required = False,
        ),
    )

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
#
# `cq.COMMENT_LEVEL_*` constants define possible values for `result_visibility`
# field of luci.cq_group(...):
#   * **cq.COMMENT_LEVEL_UNSET**: Equivalent to cq.COMMENT_LEVEL_FULL for now.
#   * **cq.COMMENT_LEVEL_FULL**: The CQ reports the summary markdown and a link
#     to the buildbucket build id in Milo with the builder name in the URL in a
#     Gerrit comment.
#   * **cq.COMMENT_LEVEL_RESTRICTED**: The CQ reports a generic "Build failed:
#     https://ci.chromium.org/b/1234" with no summary markdown.
#
# `cq.MODE_*` constants define common values for cq run modes.
#   * **cq.MODE_DRY_RUN**: Run all tests but do not submit.
#   * **cq.MODE_QUICK_DRY_RUN**: Run some tests but do not submit.
#     See https://crbug.com/1189817. To show Quick Run status in Gerrit, set the
#     `rts_was_used` output property to true. For more, see the
#     [Gerrit plugin](https://chromium.googlesource.com/infra/gerrit-plugins/buildbucket/+/refs/heads/main/README.md).
#   * **cq.MODE_FULL_RUN**: Run all tests and potentially submit.
#   * **cq.MODE_ANALYZER_RUN**: Run code analyzers on patchset upload.
#     As of April 2021, all such runs are launched by Tricium. Eventually,
#     Change Verifier(CV) will launch and manage all analyzer runs.
cq = struct(
    refset = _refset,
    retry_config = _retry_config,
    run_mode = _run_mode,
    location_filter = _location_filter,
    quota_policy = _quota_policy,
    run_limits = _run_limits,
    tryjob_limits = _tryjob_limits,
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
    COMMENT_LEVEL_UNSET = cq_pb.COMMENT_LEVEL_UNSET,
    COMMENT_LEVEL_FULL = cq_pb.COMMENT_LEVEL_FULL,
    COMMENT_LEVEL_RESTRICTED = cq_pb.COMMENT_LEVEL_RESTRICTED,
    MODE_DRY_RUN = "DRY_RUN",
    MODE_QUICK_DRY_RUN = "QUICK_DRY_RUN",
    MODE_FULL_RUN = "FULL_RUN",
    MODE_ANALYZER_RUN = "ANALYZER_RUN",
)

cqimpl = struct(
    validate_refset = _validate_refset,
    validate_retry_config = _validate_retry_config,
    validate_run_mode = _validate_run_mode,
    validate_location_filter = _validate_location_filter,
    validate_quota_policy = _validate_quota_policy,
)
