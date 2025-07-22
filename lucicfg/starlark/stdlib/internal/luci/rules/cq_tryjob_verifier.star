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

"""Defines luci.cq_tryjob_verifier(...) rule."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "keys", "kinds")
load("@stdlib//internal/luci/lib/cq.star", "cq", "cqimpl")

def _cq_tryjob_verifier(
        ctx,  # @unused
        builder = None,
        *,
        cq_group = None,
        includable_only = None,
        result_visibility = None,
        disable_reuse = None,
        disable_reuse_footers = None,
        cancel_stale = None,
        experiment_percentage = None,
        location_filters = None,
        owner_whitelist = None,
        equivalent_builder = None,
        equivalent_builder_percentage = None,
        equivalent_builder_whitelist = None,
        mode_allowlist = None):
    """A verifier in a luci.cq_group(...) that triggers tryjobs to verify CLs.

    When processing a CL, the CQ examines a list of registered verifiers and
    launches new corresponding builds (called "tryjobs") if it decides this is
    necessary (per the configuration of the verifier and the previous history
    of this CL).

    The CQ automatically retries failed tryjobs (per configured `retry_config`
    in luci.cq_group(...)) and only allows CL to land if each builder has
    succeeded in the latest retry. If a given tryjob result is too old (>1 day)
    it is ignored.

    #### Filtering based on files touched by a CL

    The CQ can examine a set of files touched by the CL and decide to skip this
    verifier. Touching a file means either adding, modifying or removing it.

    This is controlled by the `location_filters` field.

    location_filters is a list of filters, each of which includes regular
    expressions for matching Gerrit host, project, and path. The Gerrit host,
    Gerrit project and file path for each file in each CL are matched against
    the filters; The last filter that matches all paterns determines whether
    the file is considered included (not skipped) or excluded (skipped); if the
    last matching LocationFilter has exclude set to true, then the builder is
    skipped. If none of the LocationFilters match, then the file is considered
    included if the first rule is an exclude rule; else the file is excluded.

    The comparison is a full match. The pattern is implicitly anchored with `^`
    and `$`, so there is no need add them. The pattern must use [Google
    Re2](https://github.com/google/re2) library syntax, [documented
    here](https://github.com/google/re2/wiki/Syntax).

    This filtering currently cannot be used in any of the following cases:

      * For verifiers in CQ groups with `allow_submit_with_open_deps = True`.

    Please talk to CQ owners if these restrictions are limiting you.

    ##### Examples

    Enable the verifier only for all CLs touching any file in `third_party/blink`
    directory of the main branch of `chromium/src` repo.

        luci.cq_tryjob_verifier(
            location_filters = [
                cq.location_filter(
                    gerrit_host_regexp = 'chromium-review.googlesource.com',
                    gerrit_project_regexp = 'chromium/src'
                    gerrit_ref_regexp = 'refs/heads/main'
                    path_regexp = 'third_party/blink/.+')
            ],
        )

    Enable the verifier for CLs that touch files in "foo/", on any host and repo.

        luci.cq_tryjob_verifier(
            location_filters = [
                cq.location_filter(path_regexp = 'foo/.+')
            ],
        )

    Disable the verifier for CLs that *only* touches the "all/one.txt" file in
    "repo" of "example.com". If the CL touches anything else in the same host
    and repo, or touches any file in a different repo and/or host, the verifier
    will be enabled.

        luci.cq_tryjob_verifier(
            location_filters = [
                cq.location_filter(
                    gerrit_host_regexp = 'example.com',
                    gerrit_project_regexp = 'repo',
                    path_regexp = 'all/one.txt',
                    exclude = True),
            ],
        )

    Match a CL which touches at least one file other than `one.txt` inside
    `all/` directory of the Gerrit project `repo`:

        luci.cq_tryjob_verifier(
            location_filters = [
                cq.location_filter(
                    gerrit_host_regexp = 'example.com',
                    gerrit_project_regexp = 'repo',
                    path_regexp = 'all/.+'),
                cq.location_filter(
                    gerrit_host_regexp = 'example.com',
                    gerrit_project_regexp = 'repo',
                    path_regexp = 'all/one.txt',
                    exclude = True),
            ],
        )

    #### Per-CL opt-in only builders

    For builders which may be useful only for some CLs, predeclare them using
    `includable_only=True` flag. Such builders will be triggered by CQ if and
    only if a CL opts in via `CQ-Include-Trybots: <builder>` in its description.

    For example, default verifiers may include only fast builders which skip low
    level assertions, but for coverage of such assertions one may add slower
    "debug" level builders into which CL authors opt-in as needed:

          # triggered & required for all CLs.
          luci.cq_tryjob_verifier(builder="win")
          # triggered & required if only if CL opts in via
          # `CQ-Include-Trybots: project/try/win-debug`.
          luci.cq_tryjob_verifier(builder="win-debug", includable_only=True)

    #### Declaring verifiers

    `cq_tryjob_verifier` is used inline in luci.cq_group(...) declarations to
    provide per-builder verifier parameters. `cq_group` argument can be omitted
    in this case:

        luci.cq_group(
            name = 'Main CQ',
            ...
            verifiers = [
                luci.cq_tryjob_verifier(
                    builder = 'Presubmit',
                    disable_reuse = True,
                ),
                ...
            ],
        )


    It can also be associated with a luci.cq_group(...) outside of
    luci.cq_group(...) declaration. This is in particular useful in functions.
    For example:

        luci.cq_group(name = 'Main CQ')

        def try_builder(name, ...):
            luci.builder(name = name, ...)
            luci.cq_tryjob_verifier(builder = name, cq_group = 'Main CQ')

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      builder: a builder to launch when verifying a CL, see luci.builder(...).
        Can also be a reference to a builder defined in another project. See
        [Referring to builders in other projects](#external-builders) for more
        details. Required.
      cq_group: a CQ group to add the verifier to. Can be omitted if
        `cq_tryjob_verifier` is used inline inside some luci.cq_group(...)
        declaration.
      result_visibility: can be used to restrict the visibility of the tryjob
        results in comments on Gerrit. Valid values are `cq.COMMENT_LEVEL_FULL`
        and `cq.COMMENT_LEVEL_RESTRICTED` constants. Default is to give full
        visibility: builder name and full summary markdown are included in the
        Gerrit comment.
      cancel_stale: Controls whether not yet finished builds previously
        triggered by CQ will be cancelled as soon as a substantially different
        patchset is uploaded to a CL. Default is True, meaning CQ will cancel.
        In LUCI Change Verifier (aka CV, successor of CQ), changing this
        option will only take effect on newly-created Runs once config
        propagates to CV. Ongoing Runs will retain the old behavior.
        (TODO(crbug/1127991): refactor this doc after migration. As of 09/2020,
        CV implementation is WIP)
      includable_only: if True, this builder will only be triggered by CQ if it
        is also specified via `CQ-Include-Trybots:` on CL description. Default
        is False. See the explanation above for all details. For builders with
        `experiment_percentage` or `location_filters`, don't specify
        `includable_only`. Such builders can already be forcefully added via
        `CQ-Include-Trybots:` in the CL description.
      disable_reuse: if True, a fresh build will be required for each CQ
        attempt. Default is False, meaning the CQ may re-use a successful build
        triggered before the current CQ attempt started. This option is
        typically used for verifiers which run presubmit scripts, which are
        supposed to be quick to run and provide additional OWNERS, lint, etc.
        checks which are useful to run against the latest revision of the CL's
        target branch.
      disable_reuse_footers: a list of footers for which previous CQ attempts
        will be not be reused if the footer is added, removed, or has its value
        changed. Cannot be used together with `disable_reuse = True`, which
        unconditionally disables reuse.
      experiment_percentage: when this field is present, it marks the verifier
        as experimental. Such verifier is only triggered on a given percentage
        of the CLs and the outcome does not affect the decision whether a CL can
        land or not. This is typically used to test new builders and estimate
        their capacity requirements.
      location_filters: a list of cq.location_filter(...).
      owner_whitelist: a list of groups with accounts of CL owners to enable
        this builder for. If set, only CLs owned by someone from any one of
        these groups will be verified by this builder. Any CL owner outside of
        the groups that opts their CL into the builder via `CQ-Include-Trybots:`
        will be met with an error from CV.
      equivalent_builder: an optional alternative builder for the CQ to choose
        instead. If provided, the CQ will choose only one of the equivalent
        builders as required based purely on the given CL and CL's owner and
        **regardless** of the possibly already completed try jobs.
      equivalent_builder_percentage: a percentage expressing probability of the
        CQ triggering `equivalent_builder` instead of `builder`. A choice itself
        is made deterministically based on CL alone, hereby all CQ attempts on
        all patchsets of a given CL will trigger the same builder, assuming CQ
        config doesn't change in the mean time. Note that if
        `equivalent_builder_whitelist` is also specified, the choice over which
        of the two builders to trigger will be made only for CLs owned by the
        accounts in the whitelisted group. Defaults to 0, meaning the equivalent
        builder is never triggered by the CQ, but an existing build can be
        re-used.
      equivalent_builder_whitelist: a group name with accounts to enable the
        equivalent builder substitution for. If set, only CLs that are owned
        by someone from this group have a chance to be verified by the
        equivalent builder. All other CLs are verified via the main builder.
      mode_allowlist: a list of modes that CQ will trigger this verifier for.
        CQ supports `cq.MODE_DRY_RUN` and `cq.MODE_FULL_RUN`, and
        `cq.MODE_NEW_PATCHSET_RUN` out of the box.
        Additional Run modes can be defined via
        `luci.cq_group(additional_modes=...)`.
    """
    builder = keys.builder_ref(builder, attr = "builder", allow_external = True)

    location_filters = validate.list("location_filters", location_filters)
    for lf in location_filters:
        cqimpl.validate_location_filter("location_filters", lf)

    owner_whitelist = validate.list("owner_whitelist", owner_whitelist)
    for o in owner_whitelist:
        validate.string("owner_whitelist", o)

    # 'equivalent_builder' has same format as 'builder', except it is optional.
    if equivalent_builder:
        equivalent_builder = keys.builder_ref(
            equivalent_builder,
            attr = "equivalent_builder",
            allow_external = True,
        )

    equivalent_builder_percentage = validate.float(
        "equivalent_builder_percentage",
        equivalent_builder_percentage,
        min = 0.0,
        max = 100.0,
        required = False,
    )
    equivalent_builder_whitelist = validate.string(
        "equivalent_builder_whitelist",
        equivalent_builder_whitelist,
        required = False,
    )

    mode_allowlist = validate.list("mode_allowlist", mode_allowlist)
    for m in mode_allowlist:
        validate.string("mode_allowlist", m)

    if disable_reuse and disable_reuse_footers:
        fail('"disable_reuse" and "disable_reuse_footers" can not be used together')

    if not equivalent_builder:
        if equivalent_builder_percentage != None:
            fail('"equivalent_builder_percentage" can be used only together with "equivalent_builder"')
        if equivalent_builder_whitelist != None:
            fail('"equivalent_builder_whitelist" can be used only together with "equivalent_builder"')

    if includable_only:
        if location_filters:
            fail('"includable_only" can not be used together with "location_filters"')
        if experiment_percentage:
            fail('"includable_only" can not be used together with "experiment_percentage"')
        if mode_allowlist:
            fail('"includable_only" can not be used together with "mode_allowlist"')

    # Note: The name of this node is important only for error messages. It
    # doesn't show up in any generated files, and by construction it can't
    # accidentally collide with some other name.
    key = keys.unique(kinds.CQ_TRYJOB_VERIFIER, builder.id)
    graph.add_node(key, props = {
        "disable_reuse": validate.bool("disable_reuse", disable_reuse, required = False),
        "disable_reuse_footers": validate.list("disable_reuse_footers", disable_reuse_footers, required = False),
        "result_visibility": validate.int(
            "result_visibility",
            result_visibility,
            default = cq.COMMENT_LEVEL_UNSET,
            required = False,
        ),
        "cancel_stale": validate.bool("cancel_stale", cancel_stale, required = False),
        "includable_only": validate.bool("includable_only", includable_only, required = False),
        "experiment_percentage": validate.float(
            "experiment_percentage",
            experiment_percentage,
            min = 0.0,
            max = 100.0,
            required = False,
        ),
        "location_filters": location_filters,
        "owner_whitelist": owner_whitelist,
        "mode_allowlist": mode_allowlist,
    })
    if cq_group:
        graph.add_edge(parent = keys.cq_group(cq_group), child = key)
    graph.add_edge(parent = key, child = builder)

    # Need to setup a node to represent 'equivalent_builder' so that lucicfg can
    # verify (via the graph integrity check) that such builder was actually
    # defined somewhere. Note that we can't add 'equivalent_builder' as another
    # child of 'cq_tryjob_verifier' node, since then it would be ambiguous which
    # child builder_ref node is the "main one" and which is the equivalent.
    if equivalent_builder:
        # Note: this key is totally invisible.
        eq_key = keys.unique(
            kind = kinds.CQ_EQUIVALENT_BUILDER,
            name = equivalent_builder.id,
        )
        graph.add_node(eq_key, props = {
            "percentage": equivalent_builder_percentage,
            "whitelist": equivalent_builder_whitelist,
        })
        graph.add_edge(parent = key, child = eq_key)
        graph.add_edge(parent = eq_key, child = equivalent_builder)

    # This is used to detect cq_tryjob_verifier nodes that aren't connected to
    # any cq_group. Such orphan nodes aren't allowed.
    graph.add_node(keys.cq_verifiers_root(), idempotent = True)
    graph.add_edge(parent = keys.cq_verifiers_root(), child = key)

    return graph.keyset(key)

cq_tryjob_verifier = lucicfg.rule(impl = _cq_tryjob_verifier)
