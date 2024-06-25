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

"""Defines luci.cq_group(...) rule."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "keys", "kinds")
load("@stdlib//internal/luci/lib/acl.star", "acl", "aclimpl")
load("@stdlib//internal/luci/lib/cq.star", "cq", "cqimpl")
load("@stdlib//internal/luci/rules/cq_tryjob_verifier.star", "cq_tryjob_verifier")

def _cq_group(
        ctx,  # @unused
        *,
        name = None,
        watch = None,
        acls = None,
        allow_submit_with_open_deps = None,
        allow_owner_if_submittable = None,
        trust_dry_runner_deps = None,
        allow_non_owner_dry_runner = None,
        tree_status_host = None,
        tree_status_name = None,
        retry_config = None,
        cancel_stale_tryjobs = None,  # @unused
        verifiers = None,
        additional_modes = None,
        user_limits = None,
        user_limit_default = None,
        post_actions = None,
        tryjob_experiments = None):
    """Defines a set of refs to watch and a set of verifier to run.

    The CQ will run given verifiers whenever there's a pending approved CL for
    a ref in the watched set.

    Pro-tip: a command line tool exists to validate a locally generated .cfg
    file and verify that it matches arbitrary given CLs as expected.
    See https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/cv/#luci-cv-command-line-utils

    **NOTE**: if you are configuring a luci.cq_group for a new Gerrit host,
    follow instructions at http://go/luci/cv/gerrit-pubsub to ensure that
    pub/sub integration is enabled for the Gerrit host.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      name: a human- and machine-readable name this CQ group. Must be unique
        within this project. This is used in messages posted to users and in
        monitoring data. Must match regex `^[a-zA-Z][a-zA-Z0-9_-]*$`.
      watch: either a single cq.refset(...) or a list of cq.refset(...) (one per
        repo), defining what set of refs the CQ should monitor for pending CLs.
        Required.
      acls: list of acl.entry(...) objects with ACLs specific for this CQ group.
        Only `acl.CQ_*` roles are allowed here. By default ACLs are inherited
        from luci.project(...) definition. At least one `acl.CQ_COMMITTER` entry
        should be provided somewhere (either here or in luci.project(...)).
      allow_submit_with_open_deps: controls how a CQ full run behaves when the
        current Gerrit CL has open dependencies (not yet submitted CLs on which
        *this* CL depends). If set to False (default), the CQ will abort a full
        run attempt immediately if open dependencies are detected. If set to
        True, then the CQ will not abort a full run, and upon passing all other
        verifiers, the CQ will attempt to submit the CL regardless of open
        dependencies and whether the CQ verified those open dependencies. In
        turn, if the Gerrit project config allows this, Gerrit will submit all
        dependent CLs first and then this CL.
      allow_owner_if_submittable: allow CL owner to trigger CQ after getting
        `Code-Review` and other approvals regardless of `acl.CQ_COMMITTER` or
        `acl.CQ_DRY_RUNNER` roles. Only `cq.ACTION_*` are allowed here. Default
        is `cq.ACTION_NONE` which grants no additional permissions. CL owner is
        user owning a CL, i.e. its first patchset uploader, not to be confused
        with OWNERS files. **WARNING**: using this option is not recommended if
        you have sticky `Code-Review` label because this allows a malicious
        developer to upload a good looking patchset at first, get code review
        approval, and then upload a bad patchset and CQ it right away.
      trust_dry_runner_deps: consider CL dependencies that are owned by members
        of the `acl.CQ_DRY_RUNNER` role as trusted, even if they are not
        approved. By default, unapproved dependencies are only trusted if they
        are owned by members of the `acl.CQ_COMMITER` role. This allows CQ dry
        run on CLs with unapproved dependencies owned by members of
        `acl.CQ_DRY_RUNNER` role.
      allow_non_owner_dry_runner: allow members of the `acl.CQ_DRY_RUNNER` role
        to trigger DRY_RUN CQ on CLs that are owned by someone else, if all the
        CL dependencies are trusted.
      tree_status_host: **Deprecated**. Please use tree_status_name instead. A
        hostname of the project tree status app (if any). It is used by the CQ
        to check the tree status before committing a CL. If the tree is closed,
        then the CQ will wait until it is reopened.
      tree_status_name: the name of the tree that gates CL submission. If the
        tree is closed, CL will NOT be submitted until the tree is reopened.
        The tree status UI is at
        https://ci.chromium.org/ui/labs/tree-status/<tree_status_name>.
      retry_config: a new cq.retry_config(...) struct or one of `cq.RETRY_*`
        constants that define how CQ should retry failed builds. See
        [CQ](#cq-doc) for more info. Default is `cq.RETRY_TRANSIENT_FAILURES`.
      cancel_stale_tryjobs: unused anymore, but kept for backward compatibility.
      verifiers: a list of luci.cq_tryjob_verifier(...) specifying what checks
        to run on a pending CL. See luci.cq_tryjob_verifier(...) for all
        details. As a shortcut, each entry can also either be a dict or a
        string. A dict is an alias for `luci.cq_tryjob_verifier(**entry)` and
        a string is an alias for `luci.cq_tryjob_verifier(builder = entry)`.
      additional_modes: either a single cq.run_mode(...) or a list of
        cq.run_mode(...) defining additional run modes supported by this CQ
        group apart from standard DRY_RUN and FULL_RUN. If specified, CQ will
        create the Run with the first mode for which triggering conditions are
        fulfilled. If there is no such mode, CQ will fallback to standard
        DRY_RUN or FULL_RUN.
      user_limits: a list of cq.user_limit(...) or None. **WARNING**: Please
        contact luci-eng@ before setting this param. They specify per-user
        limits/quotas for given principals. At the time of a Run start, CV looks
        up and applies the first matching cq.user_limit(...) to the Run, and
        postpones the start if limits were reached already. If none of the
        user_limit(s) were applicable, `user_limit_default` will be applied
        instead. Each cq.user_limit(...) must specify at least one user or
        group.
      user_limit_default: cq.user_limit(...) or None. **WARNING*:: Please
        contact luci-eng@ before setting this param. If none of limits in
        `user_limits` are applicable and `user_limit_default` is not specified,
        the user is granted unlimited runs and tryjobs. `user_limit_default`
        must not specify users and groups.
      post_actions: a list of post actions or None.
        Please refer to cq.post_action_* for all the available post actions.
        e.g., cq.post_action_gerrit_label_votes(...)
      tryjob_experiments: a list of cq.tryjob_experiment(...) or None. The
        experiments will be enabled when launching Tryjobs if condition is met.
    """
    key = keys.cq_group(validate.string("name", name))

    # Accept cq.refset passed as is (not wrapped in a list). Most CQ configs use
    # a single cq.refset.
    if watch and type(watch) != "list":
        watch = [watch]
    for w in validate.list("watch", watch, required = True):
        cqimpl.validate_refset("watch", w)

    # Accept cq.run_mode passed as is (not wrapped in a list).
    if additional_modes:
        if type(additional_modes) != "list":
            additional_modes = [additional_modes]
        validate.list("additional_modes", additional_modes, required = False)
        for m in additional_modes:
            cqimpl.validate_run_mode("run_mode", m)

    limit_names = dict()
    user_limits = validate.list("user_limits", user_limits)
    for i, lim in enumerate(user_limits):
        lim = cqimpl.validate_user_limit("user_limits[%d]" % i, lim, required = True)
        if lim.name in limit_names:
            fail("user_limits[%d]: duplicate limit name '%s'" % (i, lim.name))
        if not lim.principals:
            fail("user_limits[%d]: must specify at least one user or group" % i)
        limit_names[lim.name] = None

    user_limit_default = cqimpl.validate_user_limit(
        "user_limit_default",
        user_limit_default,
        required = False,
    )

    # TODO(crbug.com/1346143): make user_limit_default required.
    if user_limit_default != None:
        if user_limit_default.name in limit_names:
            fail("user_limit_default: limit name '%s' is already used in user_limits" % user_limit_default.name)
        if user_limit_default.principals:
            fail("user_limit_default: must not specify user or group")

    validate.list("post_actions", post_actions, required = False)
    known_action_names = dict()
    for i, pa in enumerate(post_actions or []):
        cqimpl.validate_post_action("post_actions[%d]" % i, pa, required = True)
        if pa.name in known_action_names:
            fail("post_action[%d]: duplicate post_action name '%s'" % (i, pa.name))
        known_action_names[pa.name] = i

    validate.list("tryjob_experiments", tryjob_experiments, required = False)
    known_exp_names = dict()
    for i, te in enumerate(tryjob_experiments or []):
        cqimpl.validate_tryjob_experiment(
            "tryjob_experiments[%d]" % i,
            te,
            required = True,
        )
        if te.name in known_exp_names:
            fail("tryjob_experiments[%d]: duplicate experiment name '%s'" % (i, te.name))
        known_exp_names[te.name] = i

    if tree_status_host and tree_status_name:
        fail("tree_status_host and tree_stats_name are both set. Please unset tree_status_host.")

    # TODO(vadimsh): Convert `acls` to luci.binding(...). Need to figure out
    # what realm to use for them. This probably depends on a design of
    # Realms + CQ which doesn't exist yet.

    graph.add_node(key, props = {
        "watch": watch,
        "acls": aclimpl.validate_acls(acls, allowed_roles = [acl.CQ_COMMITTER, acl.CQ_DRY_RUNNER, acl.CQ_NEW_PATCHSET_RUN_TRIGGERER]),
        "allow_submit_with_open_deps": validate.bool(
            "allow_submit_with_open_deps",
            allow_submit_with_open_deps,
            required = False,
        ),
        "allow_owner_if_submittable": validate.int(
            "allow_owner_if_submittable",
            allow_owner_if_submittable,
            default = cq.ACTION_NONE,
            required = False,
        ),
        "trust_dry_runner_deps": validate.bool(
            "trust_dry_runner_deps",
            trust_dry_runner_deps,
            required = False,
        ),
        "allow_non_owner_dry_runner": validate.bool(
            "allow_non_owner_dry_runner",
            allow_non_owner_dry_runner,
            required = False,
        ),
        "tree_status_host": validate.hostname("tree_status_host", tree_status_host, required = False),
        "tree_status_name": validate.string("tree_status_name", tree_status_name, required = False),
        "retry_config": cqimpl.validate_retry_config(
            "retry_config",
            retry_config,
            default = cq.RETRY_TRANSIENT_FAILURES,
            required = False,
        ),
        "additional_modes": additional_modes,
        "user_limits": user_limits,
        "user_limit_default": user_limit_default,
        "post_actions": post_actions,
        "tryjob_experiments": tryjob_experiments,
    })
    graph.add_edge(keys.project(), key)

    # Add all verifiers, possibly instantiating them from dicts or direct
    # builder references (given either as strings or BUILDER_REF keysets).
    for v in validate.list("verifiers", verifiers):
        if type(v) == "dict":
            v = cq_tryjob_verifier(**v)
        elif type(v) == "string" or (graph.is_keyset(v) and v.has(kinds.BUILDER_REF)):
            v = cq_tryjob_verifier(builder = v)
        graph.add_edge(key, v.get(kinds.CQ_TRYJOB_VERIFIER))

    return graph.keyset(key)

cq_group = lucicfg.rule(impl = _cq_group)
