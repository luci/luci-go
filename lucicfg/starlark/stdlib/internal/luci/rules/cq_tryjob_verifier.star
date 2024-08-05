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
load("@stdlib//internal/re.star", "re")
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

    #### Declaring a Tricium analyzer

    `cq_tryjob_verifier` can be used to declare a [Tricium] analyzer by
    providing the builder and `mode_allowlist=[cq.MODE_ANALYZER_RUN]`. It will
    generate the Tricium config as well as CQ config, so that no additional
    changes should be required as Tricium is merged into CV.

    However, the following restrictions apply until CV takes on Tricium:

    * Most CQ features are not supported except for `location_filters` and
      `owner_whitelist`. If provided, they must meet the following conditions:
        * `location_filters` must specify either both host_regexp and
          project_regexp or neither. For path_regexp, it must match file
          extension only (e.g. `.+\\.py`) or everything. Note that, the exact
          same set of Gerrit repos should be specified across all analyzers in
          this cq_group and across each unique file extension.
        * `owner_whitelist` must be the same for all analyzers declared
          in this cq_group.
    * Analyzers will run on changes targeting **all refs** of the Gerrit repos
      watched by the containing cq_group (or repos derived from
      location_filters, see above) even though refs or refs_exclude may be
      provided.
    * All analyzers must be declared in a single luci.cq_group(...).

    For example:

        luci.project(tricium="tricium-prod.appspot.com")

        luci.cq_group(
            name = 'Main CQ',
            ...
            verifiers = [
                luci.cq_tryjob_verifier(
                    builder = "spell-checker",
                    owner_whitelist = ["project-committer"],
                    mode_allowlist = [cq.MODE_ANALYZER_RUN],
                ),
                luci.cq_tryjob_verifier(
                    builder = "go-linter",
                    location_filters = [cq.location_filter(path_regexp = ".+\\.go")]
                    owner_whitelist = ["project-committer"],
                    mode_allowlist = [cq.MODE_ANALYZER_RUN],
                ),
                luci.cq_tryjob_verifier(builder = "Presubmit"),
                ...
            ],
        )

    Note for migrating to lucicfg for LUCI Projects whose sole purpose is
    to host a single Tricium config today
    ([Example](https://fuchsia.googlesource.com/infra/config/+/HEAD/repositories/infra/recipes/tricium-prod.cfg)):

    Due to the restrictions mentioned above, it is not possible to merge those
    auxiliary Projects back to the main LUCI Project. It will be unblocked
    after Tricium is folded into CV. To migrate, users can declare new
    luci.cq_group(...)s in those Projects to host Tricium analyzers. However,
    CQ config should not be generated because the config groups will overlap
    with the config group in the main LUCI Project (i.e. watch same refs) and
    break CQ. This can be done by asking lucicfg to track only Tricium config:
    `lucicfg.config(tracked_files=["tricium-prod.cfg"])`.

    [Tricium]: https://chromium.googlesource.com/infra/infra/+/HEAD/go/src/infra/tricium

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
      experiment_percentage: when this field is present, it marks the verifier
        as experimental. Such verifier is only triggered on a given percentage
        of the CLs and the outcome does not affect the decision whether a CL can
        land or not. This is typically used to test new builders and estimate
        their capacity requirements.
      location_filters: a list of cq.location_filter(...).
      owner_whitelist: a list of groups with accounts of CL owners to enable
        this builder for. If set, only CLs owned by someone from any one of
        these groups will be verified by this builder.
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

    # Validate location_filters used by analyzers.
    # TODO(crbug/1202952): Remove these restrictions after Tricium is
    # folded into CV.
    if cq.MODE_ANALYZER_RUN in mode_allowlist:
        _validate_analyzer_location(location_filters)

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

def _validate_analyzer_location(location_filters):
    """Validates location_filters for analyzers.

    Some parts of Tricium config are generated from location_filters. But
    because of the way that Tricium watches one set of repos per config and
    uses glob path filters which (in practice) are used for file extensions,
    not all location filters are valid for analyzers.

    Specifically: Since all analyzers in a Tricium config are watching the same
    set of Gerrit repos, we need to make sure that for each extension this
    analyzer is watching, it MUST specify the same set of Gerrit repos it is
    watching. This allows lucicfg to derive a homogeneous set of watching
    Gerrit repos when generating Tricium config later.

    For example, location_filters values that match repo1 and repo2 with path
    filter *.go; and only repo1 with path filter .*.py would not be allowed,
    because the generated Tricium config has to watch both repo1 and repo2. If
    we allow it, Tricium will implicitly run for Python files in repo2 which is
    not what user intended.
    """
    if not location_filters:
        return

    re_for_ext_re = r"\.\+\\\.\w+"
    ext_prefix = r".+\."

    def matches_ext(s):
        return re.submatches(re_for_ext_re, s) and s.startswith(ext_prefix)

    re_for_gerrit_host_re = r"[a-z\-]+\-review\.googlesource\.com"
    re_for_gerrit_project_re = r"[a-z0-9\.\-/]+"

    ext_to_gerrit_urls = {}
    all_gerrit_urls = []

    for f in location_filters:
        if f.exclude:
            fail('"analyzer currently can not be used together with exclude filters')

        # Path filter must be empty (matching everything) or match only an extension.
        empty_path = f.path_regexp in ("", ".*", ".+")
        if not empty_path and not matches_ext(f.path_regexp):
            fail('"location_filter" of an analyzer MUST have a path_regexp ' +
                 'that matches everything, OR a path_regexp like ".+\\.py"; ' +
                 'got "%s", expecting pattern "%s"' % (f.path_regexp, re_for_ext_re))
        ext = ""
        if matches_ext(f.path_regexp):
            ext = f.path_regexp[len(ext_prefix):]

        empty_host = f.gerrit_host_regexp in ("", ".*", ".+")
        empty_project = f.gerrit_project_regexp in ("", ".*", ".+")
        if (not empty_host and empty_project) or (empty_host and not empty_project):
            # Only host or project specified, not both.
            fail('"location_filter" of an analyzer MUST have either both Gerrit host and project ' +
                 'or neither. Got "%s", "%s"' % (f.gerrit_host_regexp, f.gerrit_project_regexp))

        # gerrit_url below is a combination of host and project; both must be
        # specified and match the expected formats.
        gerrit_url = ""
        if not empty_host and not empty_project:
            gerrit_url = f.gerrit_host_regexp + "/" + f.gerrit_project_regexp
            if not re.submatches(re_for_gerrit_host_re, f.gerrit_host_regexp):
                fail("Gerrit host in location filter did not match expected format, " +
                     'got "%s", expecting pattern "%s"' % (f.gerrit_host_regexp, re_for_gerrit_host_re))
            if not re.submatches(re_for_gerrit_project_re, f.gerrit_project_regexp):
                fail("Gerrit project in location filter did not match expected format, " +
                     'got "%s", expecting pattern "%s"' % (f.gerrit_project_regexp, re_for_gerrit_project_re))

        if ext not in ext_to_gerrit_urls:
            ext_to_gerrit_urls[ext] = []
        if ((gerrit_url and "" in all_gerrit_urls) or (gerrit_url == "" and any(all_gerrit_urls))):
            fail(r'"location_filters" of an analyzer MUST NOT mix two different formats ' +
                 r'(i.e. only extension, and extension plus gerrit host/project."')
        ext_to_gerrit_urls[ext].append(gerrit_url)
        all_gerrit_urls.append(gerrit_url)

    ref_ext, ref_gerrit_urls = ext_to_gerrit_urls.popitem()
    ref_gerrit_urls = sorted(ref_gerrit_urls)
    for ext, gerrit_urls in ext_to_gerrit_urls.items():
        if sorted(gerrit_urls) != ref_gerrit_urls:
            fail('each extension specified in "location_filters" of an analyzer ' +
                 "MUST have the same set of gerrit URLs; " +
                 "got %s for extension %s, but %s for extension %s." % (
                     sorted(gerrit_urls),
                     ext,
                     ref_gerrit_urls,
                     ref_ext,
                 ))

cq_tryjob_verifier = lucicfg.rule(impl = _cq_tryjob_verifier)
