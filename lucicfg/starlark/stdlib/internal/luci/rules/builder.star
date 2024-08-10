# Copyright 2018 The LUCI Authors.
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

"""Defines luci.builder(...) rule."""

load("@stdlib//internal/experiments.star", "experiments")
load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "builder_ref", "keys", "triggerer")
load("@stdlib//internal/luci/lib/buildbucket.star", "buildbucket")
load("@stdlib//internal/luci/lib/resultdb.star", "resultdb", "resultdbimpl")
load("@stdlib//internal/luci/lib/scheduler.star", "schedulerimpl")
load("@stdlib//internal/luci/lib/swarming.star", "swarming")
load("@stdlib//internal/luci/rules/binding.star", "binding")
load("@stdlib//internal/luci/rules/bucket_constraints.star", "bucket_constraints")

def _generate_builder(
        ctx,  # @unused
        *,
        name = None,
        bucket = None,
        description_html = None,

        # Execution environment parameters.
        properties = None,
        allowed_property_overrides = None,
        service_account = None,
        caches = None,
        execution_timeout = None,
        grace_period = None,
        heartbeat_timeout = None,

        # Scheduling parameters.
        dimensions = None,
        priority = None,
        swarming_host = None,
        swarming_tags = None,
        expiration_timeout = None,
        wait_for_capacity = None,
        retriable = None,

        # LUCI Scheduler parameters.
        schedule = None,
        triggering_policy = None,

        # Tweaks.
        build_numbers = None,
        experimental = None,
        experiments = None,
        task_template_canary_percentage = None,
        repo = None,

        # Results.
        resultdb_settings = None,
        test_presentation = None,

        # TaskBackend.
        backend = None,
        backend_alt = None,

        # led build adjustments.
        shadow_service_account = None,
        shadow_pool = None,
        shadow_properties = None,
        shadow_dimensions = None,

        # Builder health indicators
        contact_team_email = None,

        # Custom metrics
        custom_metrics = None,

        # Dynamic builder template.
        dynamic = False):
    """Helper function for defining a generic builder.

    Shared function by luci.builder(...) and luci.dynamic_builder_template(...).

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      name: name of the builder, will show up in UIs and logs.
      Required if `dynamic` is False.
      bucket: a bucket the builder is in, see luci.bucket(...) rule. Required.
      description_html: description of the builder, will show up in UIs. See
        https://pkg.go.dev/go.chromium.org/luci/common/data/text/sanitizehtml
        for the list of allowed HTML elements.
      properties: a dict with string keys and JSON-serializable values, defining
        properties to pass to the executable. Supports the module-scoped
        defaults. They are merged (non-recursively) with the explicitly passed
        properties.
      allowed_property_overrides: a list of top-level property keys that can
        be overridden by users calling the buildbucket ScheduleBuild RPC. If
        this is set exactly to ['*'], ScheduleBuild is allowed to override
        any properties. Only property keys which are populated via the `properties`
        parameter here (or via the module-scoped defaults) are allowed.
      service_account: an email of a service account to run the executable
        under: the executable (and various tools it calls, e.g. gsutil) will be
        able to make outbound HTTP calls that have an OAuth access token
        belonging to this service account (provided it is registered with LUCI).
        Supports the module-scoped default.
      caches: a list of swarming.cache(...) objects describing Swarming named
        caches that should be present on the bot. See swarming.cache(...) doc
        for more details. Supports the module-scoped defaults. They are joined
        with the explicitly passed caches.
      execution_timeout: how long to wait for a running build to finish before
        forcefully aborting it and marking the build as timed out. If None,
        defer the decision to Buildbucket service. Supports the module-scoped
        default.
      grace_period: how long to wait after the expiration of `execution_timeout`
        or after a Cancel event, before the build is forcefully shut down. Your
        build can use this time as a 'last gasp' to do quick actions like
        killing child processes, cleaning resources, etc. Supports the
        module-scoped default.
      heartbeat_timeout: How long Buildbucket should wait for a running build to
        send any updates before forcefully fail it with `INFRA_FAILURE`. If
        None, Buildbucket won't check the heartbeat timeout. This field only
        takes effect for builds that don't have Buildbucket managing their
        underlying backend tasks, namely the ones on TaskBackendLite. E.g.
        builds running on Swarming don't need to set this.

      dimensions: a dict with swarming dimensions, indicating requirements for
        a bot to execute the build. Keys are strings (e.g. `os`), and values
        are either strings (e.g. `Linux`), swarming.dimension(...) objects (for
        defining expiring dimensions) or lists of thereof. Supports the
        module-scoped defaults. They are merged (non-recursively) with the
        explicitly passed dimensions.
      priority: int [1-255] or None, indicating swarming task priority, lower is
        more important. If None, defer the decision to Buildbucket service.
        Supports the module-scoped default.
      swarming_host: appspot hostname of a Swarming service to use for this
        builder instead of the default specified in luci.project(...). Use with
        great caution. Supports the module-scoped default.
      swarming_tags: Deprecated. Used only to enable
        "vpython:native-python-wrapper" and does not actually propagate to
        Swarming. A list of tags (`k:v` strings).
      expiration_timeout: how long to wait for a build to be picked up by a
        matching bot (based on `dimensions`) before canceling the build and
        marking it as expired. If None, defer the decision to Buildbucket
        service. Supports the module-scoped default.
      wait_for_capacity: tell swarming to wait for `expiration_timeout` even if
        it has never seen a bot whose dimensions are a superset of the requested
        dimensions. This is useful if this builder has bots whose dimensions
        are mutated dynamically. Supports the module-scoped default.
      retriable: control if the builds on the builder can be retried. Supports
        the module-scoped default.

      schedule: string with a cron schedule that describes when to run this
        builder. See [Defining cron schedules](#schedules-doc) for the expected
        format of this field. If None, the builder will not be running
        periodically.
      triggering_policy: scheduler.policy(...) struct with a configuration that
        defines when and how LUCI Scheduler should launch new builds in response
        to triggering requests from luci.gitiles_poller(...) or from
        EmitTriggers API. Does not apply to builds started directly through
        Buildbucket. By default, only one concurrent build is allowed and while
        it runs, triggering requests accumulate in a queue. Once the build
        finishes, if the queue is not empty, a new build starts right away,
        "consuming" all pending requests. See scheduler.policy(...) doc for more
        details. Supports the module-scoped default.

      build_numbers: if True, generate monotonically increasing contiguous
        numbers for each build, unique within the builder. If None, defer the
        decision to Buildbucket service. Supports the module-scoped default.
      experimental: if True, by default a new build in this builder will be
        marked as experimental. This is seen from the executable and it may
        behave differently (e.g. avoiding any side-effects). If None, defer the
        decision to Buildbucket service. Supports the module-scoped default.
      experiments: a dict that maps experiment name to percentage chance that it
        will apply to builds generated from this builder. Keys are strings,
        and values are integers from 0 to 100. This is unrelated to
        lucicfg.enable_experiment(...).
      task_template_canary_percentage: int [0-100] or None, indicating
        percentage of builds that should use a canary swarming task template.
        If None, defer the decision to Buildbucket service. Supports the
        module-scoped default.
      repo: URL of a primary git repository (starting with `https://`)
        associated with the builder, if known. It is in particular important
        when using luci.notifier(...) to let LUCI know what git history it
        should use to chronologically order builds on this builder. If unknown,
        builds will be ordered by creation time. If unset, will be taken from
        the configuration of luci.gitiles_poller(...) that trigger this builder
        if they all poll the same repo.

      resultdb_settings: A buildbucket_pb.BuilderConfig.ResultDB, such as one
        created with resultdb.settings(...). A configuration that defines if
        Buildbucket:ResultDB integration should be enabled for this builder and
        which results to export to BigQuery.
      test_presentation: A resultdb.test_presentation(...) struct. A
        configuration that defines how tests should be rendered in the UI.

      backend: the name of the task backend defined via luci.task_backend(...).
        Supports the module-scoped default.

      backend_alt: the name of the alternative task backend defined via
        luci.task_backend(...). Supports the module-scoped default.

      shadow_service_account: If set, the led builds created for this Builder
        will instead use this service account. This is useful to allow users to
        automatically have their testing builds assume a service account which
        is different than your production service account.
        When specified, the shadow_service_account will also be included into
        the shadow bucket's constraints (see luci.bucket_constraints(...)).
        Which also means it will be granted the
        `role/buildbucket.builderServiceAccount` role in the shadow bucket realm.
      shadow_pool: If set, the led builds created for this Builder will instead
        be set to use this alternate pool instead. This would allow you to grant
        users the ability to create led builds in the alternate pool without
        allowing them to create builds in the production pool.
        When specified, the shadow_pool will also be included into
        the shadow bucket's constraints (see luci.bucket_constraints(...)) and
        a "pool:<shadow_pool>" dimension will be automatically added to
        shadow_dimensions.
      shadow_properties: If set, the led builds created for this Builder will
        override the top-level input properties with the same keys.
      shadow_dimensions: If set, the led builds created for this Builder will
        override the dimensions with the same keys. Note: for historical reasons
        pool can be set individually. If a "pool:<shadow_pool>" dimension is
        included here, it would have the same effect as setting shadow_pool.
        shadow_dimensions support dimensions with None values. It's useful for
        led builds to remove some dimensions the production builds use.

      contact_team_email: the owning team's contact email. This team is responsible for fixing
        any builder health issues (see BuilderConfig.ContactTeamEmail).

      custom_metrics: a list of buildbucket.custom_metric(...) objects.
        Defines the custom metrics this builder should report to.

      dynamic: Flag for if to generate a dynamic_builder_template or a pre-defined builder.
    """
    if dynamic:
        if name != "" and name != None:
            fail("name must be unset for dynamic builder template")
    else:
        name = validate.string("name", name)
    bucket_key = keys.bucket(bucket)

    # TODO(vadimsh): Validators here and in lucicfg.rule(..., defaults = ...)
    # are duplicated. There's probably a way to avoid this by introducing a
    # Schema object.
    props = {
        "name": name,
        "bucket": bucket_key.id,
        "realm": bucket_key.id,
        "description_html": validate.string("description_html", description_html, required = False),
        "project": "",  # means "whatever is being defined right now"
        "properties": validate.str_dict("properties", properties),
        "allowed_property_overrides": validate.str_list("allowed_property_overrides", allowed_property_overrides),
        "service_account": validate.string("service_account", service_account, required = False),
        "caches": swarming.validate_caches("caches", caches),
        "execution_timeout": validate.duration("execution_timeout", execution_timeout, required = False),
        "grace_period": validate.duration("grace_period", grace_period, required = False),
        "heartbeat_timeout": validate.duration("heartbeat_timeout", heartbeat_timeout, required = False),
        "dimensions": swarming.validate_dimensions("dimensions", dimensions, allow_none = True),
        "priority": validate.int("priority", priority, min = 1, max = 255, required = False),
        "swarming_host": validate.string("swarming_host", swarming_host, required = False),
        "swarming_tags": swarming.validate_tags("swarming_tags", swarming_tags),
        "expiration_timeout": validate.duration("expiration_timeout", expiration_timeout, required = False),
        "wait_for_capacity": validate.bool("wait_for_capacity", wait_for_capacity, required = False),
        "retriable": validate.bool("retriable", retriable, required = False),
        "schedule": validate.string("schedule", schedule, required = False),
        "triggering_policy": schedulerimpl.validate_policy("triggering_policy", triggering_policy, required = False),
        "build_numbers": validate.bool("build_numbers", build_numbers, required = False),
        "experimental": validate.bool("experimental", experimental, required = False),
        "experiments": _validate_experiments("experiments", experiments, allow_none = True),
        "task_template_canary_percentage": validate.int("task_template_canary_percentage", task_template_canary_percentage, min = 0, max = 100, required = False),
        "repo": validate.repo_url("repo", repo, required = False),
        "resultdb": resultdb.validate_settings("settings", resultdb_settings),
        "test_presentation": resultdb.validate_test_presentation("test_presentation", test_presentation),
        "backend": keys.task_backend(backend) if backend != None else None,
        "backend_alt": keys.task_backend(backend_alt) if backend_alt != None else None,
        "shadow_service_account": validate.string("shadow_service_account", shadow_service_account, required = False),
        "shadow_pool": validate.string("shadow_pool", shadow_pool, required = False),
        "shadow_properties": validate.str_dict("shadow_properties", shadow_properties, required = False),
        "shadow_dimensions": swarming.validate_dimensions("shadow_dimensions", shadow_dimensions, allow_none = True),
        "contact_team_email": validate.email("contact_team_email", contact_team_email, required = False),
        "custom_metrics": buildbucket.validate_custom_metrics("custom_metrics", custom_metrics),
    }

    # Merge explicitly passed properties with the module-scoped defaults.
    for k, prop_val in props.items():
        var = getattr(ctx.defaults, k, None)
        def_val = var.get() if var else None
        if def_val == None:
            continue
        if k in ("properties", "dimensions", "experiments"):
            props[k] = _merge_dicts(def_val, prop_val)
        elif k in ("allowed_property_overrides", "caches", "swarming_tags"):
            props[k] = _merge_lists(def_val, prop_val)
        elif prop_val == None:
            props[k] = def_val

    # Check to see if allowed_property_overrides is allowing override for
    # properties not supplied.
    if props["allowed_property_overrides"] and props["allowed_property_overrides"] != ["*"]:
        # Do a de-duplication pass
        props["allowed_property_overrides"] = sorted(set(props["allowed_property_overrides"]))
        for override in props["allowed_property_overrides"]:
            if "*" in override:
                fail("allowed_property_overrides does not support wildcards: %r" % override)
            elif override not in props["properties"]:
                fail("%r listed in allowed_property_overrides but not in properties" % override)

    test_presentation = props.pop("test_presentation")

    # To reduce noise in the properties, set the test presentation config only
    # when it's not the default value.
    if test_presentation != None and test_presentation != resultdb.test_presentation():
        # Copy the properties dictionary so we won't modify the original value,
        # which could be immutable if no default value was provided.
        props["properties"] = dict(props["properties"])
        props["properties"]["$recipe_engine/resultdb/test_presentation"] = resultdbimpl.test_presentation_to_dict(test_presentation)

    # Properties and shadow_properties should be JSON-serializable.
    # The only way to check is to try to serialize. We do it here (instead of
    # generators.star) to get a more informative stack trace.
    _ = to_json(props["properties"])  # @unused
    _ = to_json(props["shadow_properties"])  # @unused

    # There should be no dimensions and experiments with value None after
    # merging.
    swarming.validate_dimensions("dimensions", props["dimensions"], allow_none = False)
    _validate_experiments("experiments", props["experiments"], allow_none = False)

    # Update shadow_pool or shadow_dimensions.
    if shadow_dimensions:
        pools_in_dimensions = [p.value for p in props["shadow_dimensions"].get("pool", [])]
        if shadow_pool:
            if len(pools_in_dimensions) > 0:
                for p in pools_in_dimensions:
                    if shadow_pool != p:
                        fail("shadow_pool and pool dimension in shadow_dimensions should have the same value")
            else:
                props["shadow_dimensions"]["pool"] = [swarming.dimension(shadow_pool)]
        elif len(pools_in_dimensions) == 1:
            props["shadow_pool"] = pools_in_dimensions[0]
    elif shadow_pool:
        props["shadow_dimensions"] = {
            "pool": [swarming.dimension(shadow_pool)],
        }

    # Setup a binding that allows the service account to be used for builds
    # in the bucket's realm.
    if props["service_account"]:
        binding(
            realm = bucket_key.id,
            roles = "role/buildbucket.builderServiceAccount",
            users = props["service_account"],
        )

    if _apply_builder_config_as_bucket_constraints.is_enabled():
        # Implicitly add constraints to this builder's bucket.
        pools = [p.value for p in props["dimensions"].get("pool", [])]
        service_accounts = [props["service_account"]] if props["service_account"] else []
        if pools or service_accounts:
            bucket_constraints(
                bucket = bucket_key.id,
                pools = pools,
                service_accounts = service_accounts,
            )
    return props

# Enables the application of a builder's config (more specifically pool and
# service_account) to its bucket as constraints.
_apply_builder_config_as_bucket_constraints = experiments.register("crbug.com/1338648")

def _builder(
        ctx,  # @unused
        *,
        name = None,
        bucket = None,
        description_html = None,
        executable = None,

        # Execution environment parameters.
        properties = None,
        allowed_property_overrides = None,
        service_account = None,
        caches = None,
        execution_timeout = None,
        grace_period = None,
        heartbeat_timeout = None,

        # Scheduling parameters.
        dimensions = None,
        priority = None,
        swarming_host = None,
        swarming_tags = None,
        expiration_timeout = None,
        wait_for_capacity = None,
        retriable = None,

        # LUCI Scheduler parameters.
        schedule = None,
        triggering_policy = None,

        # Tweaks.
        build_numbers = None,
        experimental = None,
        experiments = None,
        task_template_canary_percentage = None,
        repo = None,

        # Results.
        resultdb_settings = None,
        test_presentation = None,

        # TaskBackend.
        backend = None,
        backend_alt = None,

        # led build adjustments.
        shadow_service_account = None,
        shadow_pool = None,
        shadow_properties = None,
        shadow_dimensions = None,

        # Relations.
        triggers = None,
        triggered_by = None,
        notifies = None,

        # Builder health indicators
        contact_team_email = None,

        # Custom metrics
        custom_metrics = None):
    """Defines a generic builder.

    It runs some executable (usually a recipe) in some requested environment,
    passing it a struct with given properties. It is launched whenever something
    triggers it (a poller or some other builder, or maybe some external actor
    via Buildbucket or LUCI Scheduler APIs).

    The full unique builder name (as expected by Buildbucket RPC interface) is
    a pair `(<project>, <bucket>/<name>)`, but within a single project config
    this builder can be referred to either via its bucket-scoped name (i.e.
    `<bucket>/<name>`) or just via it's name alone (i.e. `<name>`), if this
    doesn't introduce ambiguities.

    The definition of what can *potentially* trigger what is defined through
    `triggers` and `triggered_by` fields. They specify how to prepare ACLs and
    other configuration of services that execute builds. If builder **A** is
    defined as "triggers builder **B**", it means all services should expect
    **A** builds to trigger **B** builds via LUCI Scheduler's EmitTriggers RPC
    or via Buildbucket's ScheduleBuild RPC, but the actual triggering is still
    the responsibility of **A**'s executable.

    There's a caveat though: only Scheduler ACLs are auto-generated by the
    config generator when one builder triggers another, because each Scheduler
    job has its own ACL and we can precisely configure who's allowed to trigger
    this job. Buildbucket ACLs are left unchanged, since they apply to an entire
    bucket, and making a large scale change like that (without really knowing
    whether Buildbucket API will be used) is dangerous. If the executable
    triggers other builds directly through Buildbucket, it is the responsibility
    of the config author (you) to correctly specify Buildbucket ACLs, for
    example by adding the corresponding service account to the bucket ACLs:

    ```python
    luci.bucket(
        ...
        acls = [
            ...
            acl.entry(acl.BUILDBUCKET_TRIGGERER, <builder service account>),
            ...
        ],
    )
    ```

    This is not necessary if the executable uses Scheduler API instead of
    Buildbucket.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      name: name of the builder, will show up in UIs and logs. Required.
      bucket: a bucket the builder is in, see luci.bucket(...) rule. Required.
      description_html: description of the builder, will show up in UIs. See
        https://pkg.go.dev/go.chromium.org/luci/common/data/text/sanitizehtml
        for the list of allowed HTML elements.
      executable: an executable to run, e.g. a luci.recipe(...) or
        luci.executable(...). Required.
      properties: a dict with string keys and JSON-serializable values, defining
        properties to pass to the executable. Supports the module-scoped
        defaults. They are merged (non-recursively) with the explicitly passed
        properties.
      allowed_property_overrides: a list of top-level property keys that can
        be overridden by users calling the buildbucket ScheduleBuild RPC. If
        this is set exactly to ['*'], ScheduleBuild is allowed to override
        any properties. Only property keys which are populated via the `properties`
        parameter here (or via the module-scoped defaults) are allowed.
      service_account: an email of a service account to run the executable
        under: the executable (and various tools it calls, e.g. gsutil) will be
        able to make outbound HTTP calls that have an OAuth access token
        belonging to this service account (provided it is registered with LUCI).
        Supports the module-scoped default.
      caches: a list of swarming.cache(...) objects describing Swarming named
        caches that should be present on the bot. See swarming.cache(...) doc
        for more details. Supports the module-scoped defaults. They are joined
        with the explicitly passed caches.
      execution_timeout: how long to wait for a running build to finish before
        forcefully aborting it and marking the build as timed out. If None,
        defer the decision to Buildbucket service. Supports the module-scoped
        default.
      grace_period: how long to wait after the expiration of `execution_timeout`
        or after a Cancel event, before the build is forcefully shut down. Your
        build can use this time as a 'last gasp' to do quick actions like
        killing child processes, cleaning resources, etc. Supports the
        module-scoped default.
      heartbeat_timeout: How long Buildbucket should wait for a running build to
        send any updates before forcefully fail it with `INFRA_FAILURE`. If
        None, Buildbucket won't check the heartbeat timeout. This field only
        takes effect for builds that don't have Buildbucket managing their
        underlying backend tasks, namely the ones on TaskBackendLite. E.g.
        builds running on Swarming don't need to set this.

      dimensions: a dict with swarming dimensions, indicating requirements for
        a bot to execute the build. Keys are strings (e.g. `os`), and values
        are either strings (e.g. `Linux`), swarming.dimension(...) objects (for
        defining expiring dimensions) or lists of thereof. Supports the
        module-scoped defaults. They are merged (non-recursively) with the
        explicitly passed dimensions.
      priority: int [1-255] or None, indicating swarming task priority, lower is
        more important. If None, defer the decision to Buildbucket service.
        Supports the module-scoped default.
      swarming_host: appspot hostname of a Swarming service to use for this
        builder instead of the default specified in luci.project(...). Use with
        great caution. Supports the module-scoped default.
      swarming_tags: Deprecated. Used only to enable
        "vpython:native-python-wrapper" and does not actually propagate to
        Swarming. A list of tags (`k:v` strings).
      expiration_timeout: how long to wait for a build to be picked up by a
        matching bot (based on `dimensions`) before canceling the build and
        marking it as expired. If None, defer the decision to Buildbucket
        service. Supports the module-scoped default.
      wait_for_capacity: tell swarming to wait for `expiration_timeout` even if
        it has never seen a bot whose dimensions are a superset of the requested
        dimensions. This is useful if this builder has bots whose dimensions
        are mutated dynamically. Supports the module-scoped default.
      retriable: control if the builds on the builder can be retried. Supports
        the module-scoped default.

      schedule: string with a cron schedule that describes when to run this
        builder. See [Defining cron schedules](#schedules-doc) for the expected
        format of this field. If None, the builder will not be running
        periodically.
      triggering_policy: scheduler.policy(...) struct with a configuration that
        defines when and how LUCI Scheduler should launch new builds in response
        to triggering requests from luci.gitiles_poller(...) or from
        EmitTriggers API. Does not apply to builds started directly through
        Buildbucket. By default, only one concurrent build is allowed and while
        it runs, triggering requests accumulate in a queue. Once the build
        finishes, if the queue is not empty, a new build starts right away,
        "consuming" all pending requests. See scheduler.policy(...) doc for more
        details. Supports the module-scoped default.

      build_numbers: if True, generate monotonically increasing contiguous
        numbers for each build, unique within the builder. If None, defer the
        decision to Buildbucket service. Supports the module-scoped default.
      experimental: if True, by default a new build in this builder will be
        marked as experimental. This is seen from the executable and it may
        behave differently (e.g. avoiding any side-effects). If None, defer the
        decision to Buildbucket service. Supports the module-scoped default.
      experiments: a dict that maps experiment name to percentage chance that it
        will apply to builds generated from this builder. Keys are strings,
        and values are integers from 0 to 100. This is unrelated to
        lucicfg.enable_experiment(...).
      task_template_canary_percentage: int [0-100] or None, indicating
        percentage of builds that should use a canary swarming task template.
        If None, defer the decision to Buildbucket service. Supports the
        module-scoped default.
      repo: URL of a primary git repository (starting with `https://`)
        associated with the builder, if known. It is in particular important
        when using luci.notifier(...) to let LUCI know what git history it
        should use to chronologically order builds on this builder. If unknown,
        builds will be ordered by creation time. If unset, will be taken from
        the configuration of luci.gitiles_poller(...) that trigger this builder
        if they all poll the same repo.

      resultdb_settings: A buildbucket_pb.BuilderConfig.ResultDB, such as one
        created with resultdb.settings(...). A configuration that defines if
        Buildbucket:ResultDB integration should be enabled for this builder and
        which results to export to BigQuery.
      test_presentation: A resultdb.test_presentation(...) struct. A
        configuration that defines how tests should be rendered in the UI.

      backend: the name of the task backend defined via luci.task_backend(...).
        Supports the module-scoped default.

      backend_alt: the name of the alternative task backend defined via
        luci.task_backend(...). Supports the module-scoped default.

      shadow_service_account: If set, the led builds created for this Builder
        will instead use this service account. This is useful to allow users to
        automatically have their testing builds assume a service account which
        is different than your production service account.
        When specified, the shadow_service_account will also be included into
        the shadow bucket's constraints (see luci.bucket_constraints(...)).
        Which also means it will be granted the
        `role/buildbucket.builderServiceAccount` role in the shadow bucket realm.
      shadow_pool: If set, the led builds created for this Builder will instead
        be set to use this alternate pool instead. This would allow you to grant
        users the ability to create led builds in the alternate pool without
        allowing them to create builds in the production pool.
        When specified, the shadow_pool will also be included into
        the shadow bucket's constraints (see luci.bucket_constraints(...)) and
        a "pool:<shadow_pool>" dimension will be automatically added to
        shadow_dimensions.
      shadow_properties: If set, the led builds created for this Builder will
        override the top-level input properties with the same keys.
      shadow_dimensions: If set, the led builds created for this Builder will
        override the dimensions with the same keys. Note: for historical reasons
        pool can be set individually. If a "pool:<shadow_pool>" dimension is
        included here, it would have the same effect as setting shadow_pool.
        shadow_dimensions support dimensions with None values. It's useful for
        led builds to remove some dimensions the production builds use.

      triggers: builders this builder triggers.
      triggered_by: builders or pollers this builder is triggered by.
      notifies: list of luci.notifier(...) or luci.tree_closer(...) the builder
        notifies when it changes its status. This relation can also be defined
        via `notified_by` field in luci.notifier(...) or luci.tree_closer(...).

      contact_team_email: the owning team's contact email. This team is responsible for fixing
        any builder health issues (see BuilderConfig.ContactTeamEmail).

      custom_metrics: a list of buildbucket.custom_metric(...) objects.
        Defines the custom metrics this builder should report to.
    """

    name = validate.string("name", name)
    bucket_key = keys.bucket(bucket)
    executable_key = keys.executable(executable)

    props = _generate_builder(
        ctx,
        name = name,
        bucket = bucket,
        description_html = description_html,
        properties = properties,
        allowed_property_overrides = allowed_property_overrides,
        service_account = service_account,
        caches = caches,
        execution_timeout = execution_timeout,
        grace_period = grace_period,
        heartbeat_timeout = heartbeat_timeout,

        # Scheduling parameters.
        dimensions = dimensions,
        priority = priority,
        swarming_host = swarming_host,
        swarming_tags = swarming_tags,
        expiration_timeout = expiration_timeout,
        wait_for_capacity = wait_for_capacity,
        retriable = retriable,

        # LUCI Scheduler parameters.
        schedule = schedule,
        triggering_policy = triggering_policy,

        # Tweaks.
        build_numbers = build_numbers,
        experimental = experimental,
        experiments = experiments,
        task_template_canary_percentage = task_template_canary_percentage,
        repo = repo,

        # Results.
        resultdb_settings = resultdb_settings,
        test_presentation = test_presentation,

        # TaskBackend.
        backend = backend,
        backend_alt = backend_alt,

        # led build adjustments.
        shadow_service_account = shadow_service_account,
        shadow_pool = shadow_pool,
        shadow_properties = shadow_properties,
        shadow_dimensions = shadow_dimensions,

        # Builder health indicators
        contact_team_email = contact_team_email,

        # Custom metrics
        custom_metrics = custom_metrics,
    )

    # Add a node that carries the full definition of the builder.
    builder_key = keys.builder(bucket_key.id, name)
    graph.add_node(builder_key, props = props)
    graph.add_edge(bucket_key, builder_key)
    graph.add_edge(builder_key, executable_key)
    if props["backend"]:
        graph.add_edge(builder_key, props["backend"])
    if props["backend_alt"]:
        graph.add_edge(builder_key, props["backend_alt"])

    # Allow this builder to be referenced from other nodes via its bucket-scoped
    # name and via a global (perhaps ambiguous) name. See builder_ref.add(...).
    # Ambiguity is checked during the graph traversal via
    # builder_ref.follow(...).
    builder_ref_key = builder_ref.add(builder_key)

    # Setup nodes that indicate this builder can be referenced in 'triggered_by'
    # relations (either via its bucket-scoped name or via its global name).
    triggerer_key = triggerer.add(builder_key)

    # Link to builders triggered by this builder.
    for t in validate.list("triggers", triggers):
        graph.add_edge(
            parent = triggerer_key,
            child = keys.builder_ref(t),
            title = "triggers",
        )

    # And link to nodes this builder is triggered by.
    for t in validate.list("triggered_by", triggered_by):
        graph.add_edge(
            parent = keys.triggerer(t),
            child = builder_ref_key,
            title = "triggered_by",
        )

    # Subscribe notifiers/tree closers to this builder.
    for n in validate.list("notifies", notifies):
        graph.add_edge(
            parent = keys.notifiable(n),
            child = builder_ref_key,
            title = "notifies",
        )
    return graph.keyset(builder_key, builder_ref_key, triggerer_key)

def _merge_dicts(defaults, extra):
    out = dict(defaults.items())
    for k, v in extra.items():
        if v != None:
            out[k] = v
    return out

def _merge_lists(defaults, extra):
    return defaults + extra

def _validate_experiments(attr, val, allow_none = False):
    """Validates that the value is a dict of {string: int[1-100]}

    Args:
      attr: field name with this value, for error messages.
      val: a value to validate.
      allow_none: True to also allow None as dict values.

    Returns:
      The validated dict or {}.
    """
    if val == None:
        return {}

    validate.str_dict(attr, val)

    for k, percent in val.items():
        if percent == None and allow_none:
            continue
        perc_type = type(percent)
        if perc_type != "int":
            fail("bad %r: got %s for key %s, want int from 0-100" % (attr, perc_type, k))
        if percent < 0 or percent > 100:
            fail("bad %r: %d should be between 0-100" % (attr, percent))

    return val

builder = lucicfg.rule(
    impl = _builder,
    defaults = validate.vars_with_validators({
        "properties": validate.str_dict,
        "allowed_property_overrides": validate.str_list,
        "service_account": validate.string,
        "caches": swarming.validate_caches,
        "execution_timeout": validate.duration,
        "grace_period": validate.duration,
        "heartbeat_timeout": validate.duration,
        "dimensions": swarming.validate_dimensions,
        "priority": lambda attr, val: validate.int(attr, val, min = 1, max = 255),
        "swarming_host": validate.string,
        "swarming_tags": swarming.validate_tags,
        "expiration_timeout": validate.duration,
        "wait_for_capacity": validate.bool,
        "retriable": validate.bool,
        "triggering_policy": schedulerimpl.validate_policy,
        "build_numbers": validate.bool,
        "experimental": validate.bool,
        "experiments": _validate_experiments,
        "task_template_canary_percentage": lambda attr, val: validate.int(attr, val, min = 0, max = 100),
        "resultdb": resultdb.validate_settings,
        "test_presentation": resultdb.validate_test_presentation,
        "backend": lambda _attr, val: keys.task_backend(val),
        "backend_alt": lambda _attr, val: keys.task_backend(val),
    }),
)

builderimpl = struct(
    generate_builder = _generate_builder,
)
