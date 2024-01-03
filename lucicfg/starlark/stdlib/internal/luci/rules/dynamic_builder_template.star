# Copyright 2023 The LUCI Authors.
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

"""Defines luci.dynamic_builder_template(...) rule."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/luci/common.star", "keys", "kinds")
load("@stdlib//internal/luci/rules/builder.star", "builderimpl")

def _dynamic_builder_template(
        ctx,  # @unused
        *,
        bucket = None,
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
        expiration_timeout = None,
        retriable = None,

        # Tweaks.
        experiments = None,

        # Results.
        resultdb_settings = None,
        test_presentation = None,

        # TaskBackend.
        backend = None,
        backend_alt = None,

        # Builder health indicators
        contact_team_email = None):
    """Defines a dynamic builder template for a dynamic bucket.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      bucket: a bucket the builder is in, see luci.bucket(...) rule. Required.
      executable: an executable to run, e.g. a luci.recipe(...) or
        luci.executable(...).
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
      expiration_timeout: how long to wait for a build to be picked up by a
        matching bot (based on `dimensions`) before canceling the build and
        marking it as expired. If None, defer the decision to Buildbucket
        service. Supports the module-scoped default.
      retriable: control if the builds on the builder can be retried. Supports
        the module-scoped default.

      experiments: a dict that maps experiment name to percentage chance that it
        will apply to builds generated from this builder. Keys are strings,
        and values are integers from 0 to 100. This is unrelated to
        lucicfg.enable_experiment(...).

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

      contact_team_email: the owning team's contact email. This team is responsible for fixing
        any builder health issues (see BuilderConfig.ContactTeamEmail).
    """

    bucket_key = keys.bucket(bucket)

    props = builderimpl.generate_builder(
        ctx,
        name = None,
        bucket = bucket,
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
        expiration_timeout = expiration_timeout,
        retriable = retriable,
        experiments = experiments,
        resultdb_settings = resultdb_settings,
        test_presentation = test_presentation,
        backend = backend,
        backend_alt = backend_alt,
        contact_team_email = contact_team_email,
        dynamic = True,
    )

    # Add a node that carries the full definition of the builder.
    builder_key = keys.unique(kinds.DYNAMIC_BUILDER_TEMPLATE, keys.bucket(bucket).id)
    graph.add_node(builder_key, props = props)
    graph.add_edge(bucket_key, builder_key)

    if executable != None:
        executable_key = keys.executable(executable)
        graph.add_edge(builder_key, executable_key)

    if props["backend"]:
        graph.add_edge(builder_key, props["backend"])
    if props["backend_alt"]:
        graph.add_edge(builder_key, props["backend_alt"])

    return graph.keyset(builder_key)

dynamic_builder_template = lucicfg.rule(
    impl = _dynamic_builder_template,
)
