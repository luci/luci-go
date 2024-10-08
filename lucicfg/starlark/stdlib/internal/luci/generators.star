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

"""Implementation of various LUCI *.cfg file generators."""

load("@stdlib//internal/error.star", "error")
load("@stdlib//internal/experiments.star", "experiments")
load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/strutil.star", "strutil")
load("@stdlib//internal/time.star", "time")
load("@stdlib//internal/luci/common.star", "builder_ref", "keys", "kinds", "triggerer")
load("@stdlib//internal/luci/lib/acl.star", "acl", "aclimpl")
load("@stdlib//internal/luci/lib/cq.star", "cq")
load("@stdlib//internal/luci/lib/realms.star", "realms")
load(
    "@stdlib//internal/luci/proto.star",
    "buildbucket_pb",
    "common_pb",
    "config_pb",
    "cq_pb",
    "logdog_cloud_logging_pb",
    "logdog_pb",
    "milo_pb",
    "notify_pb",
    "realms_pb",
    "scheduler_pb",
    "tricium_pb",
)
load("@proto//google/protobuf/duration.proto", duration_pb = "google.protobuf")
load("@proto//google/protobuf/wrappers.proto", wrappers_pb = "google.protobuf")

# If set, do not generate legacy Buildbucket and Scheduler ACLs.
#
# Implies generation of shorter BuildbucketTask protos and conditional bindings,
# succeeding experiment "crbug.com/1182002".
_drop_legacy_shed_bb_acls = experiments.register("crbug.com/1347252", "1.32.0")

# If set, do not populate the deprecated task_template_canary_percentage field
# of BuilderConfig, but instead add the canary_software experiment.
_use_experiment_for_task_template_canary_percentage = experiments.register("crbug.com/1496969", "1.41.0")

def _legacy_acls():
    """True to generate legacy Scheduler and Buildbucket ACLs."""
    return not _drop_legacy_shed_bb_acls.is_enabled()

def register():
    """Registers all LUCI config generator callbacks."""
    lucicfg.generator(impl = gen_project_cfg)
    lucicfg.generator(impl = gen_realms_cfg)
    lucicfg.generator(impl = gen_logdog_cfg)
    lucicfg.generator(impl = gen_buildbucket_cfg)
    lucicfg.generator(impl = gen_scheduler_cfg)
    lucicfg.generator(impl = gen_milo_cfg)
    lucicfg.generator(impl = gen_cq_cfg)
    lucicfg.generator(impl = gen_notify_cfg)
    lucicfg.generator(impl = gen_tricium_cfg)

################################################################################
## Utilities to be used from generators.

def output_path(path):
    """Returns a full path to a LUCI config in the output set.

    Args:
      path: a LUCI config path relative to LUCI config root.

    Returns:
      A path relative to the config output root.
    """
    root = get_project().props.config_dir
    if root != ".":
        path = root + "/" + path
    return path

def set_config(ctx, path, cfg):
    """Adds `cfg` as a LUCI config to the output at the given `path`.

    Args:
      ctx: the generator context.
      path: the path in the output to populate.
      cfg: a proto or a string to put in the output.
    """
    ctx.output[output_path(path)] = cfg

def get_project(required = True):
    """Returns project() node or fails if it wasn't declared.

    Args:
      required: if True, fail if the luci.project(...) wasn't defined.

    Returns:
      luci.project(...) node.
    """
    n = graph.node(keys.project())
    if not n and required:
        fail("luci.project(...) definition is missing, it is required")
    return n

def get_service(kind, why):
    """Returns service struct (see service.star), reading it from project node.

    Args:
      kind: e.g. `buildbucket`.
      why: the request why it is required, for error messages.

    Returns:
      The service struct.
    """
    proj = get_project()
    svc = getattr(proj.props, kind)
    if not svc:
        fail(
            "missing %r in luci.project(...), it is required for %s" % (kind, why),
            trace = proj.trace,
        )
    return svc

def get_bb_notification_topics():
    """Returns all defined buildbucket_notification_topic() nodes, if any."""
    return graph.children(keys.project(), kinds.BUILDBUCKET_NOTIFICATION_TOPIC)

def get_buckets():
    """Returns all defined bucket() nodes, if any."""
    return graph.children(keys.project(), kinds.BUCKET)

def get_buckets_of(nodes):
    """Returns bucket() nodes with buckets that contain given nodes.

    Nodes are expected to have 'bucket' property.
    """
    buckets = set([n.props.bucket for n in nodes])
    return [b for b in get_buckets() if b.props.name in buckets]

def get_project_acls():
    """Returns [acl.elementary] with the project-level ACLs."""
    return aclimpl.normalize_acls(get_project().props.acls)

def get_bucket_acls(bucket):
    """Returns [acl.elementary] with combined bucket and project ACLs.

    Args:
      bucket: a bucket node, as returned by e.g. get_buckets().
    """
    return aclimpl.normalize_acls(bucket.props.acls + get_project().props.acls)

def filter_acls(acls, roles):
    """Keeps only ACL entries that have any of given roles."""
    return [a for a in acls if a.role in roles]

def legacy_bucket_name(bucket_name, project_name):
    """Prefixes the bucket name with `luci.<project>.`."""
    if bucket_name.startswith("luci."):
        fail("seeing long bucket name %r, shouldn't be possible" % bucket_name)
    return "luci.%s.%s" % (project_name, bucket_name)

def optional_sec(duration):
    """duration|None => number of seconds | None."""
    return None if duration == None else duration // time.second

def optional_duration_pb(duration):
    """duration|None => duration_pb.Duration | None."""
    if duration == None:
        return None
    return duration_pb.Duration(
        seconds = duration // time.second,
        nanos = (duration % time.second) * 1000000,
    )

def optional_UInt32Value(val):
    """int|None => google.protobuf.UInt32Value."""
    return None if val == None else wrappers_pb.UInt32Value(value = val)

################################################################################
## project.cfg.

def gen_project_cfg(ctx):
    """Generates project.cfg.

    Args:
      ctx: the generator context.
    """

    # lucicfg is allowed to interpret *.star files without any actual
    # definitions. This is used in tests, for example. If there's no
    # project(...) rule, but there are some other LUCI definitions, the
    # corresponding generators will fail on their own in get_project() calls.
    proj = get_project(required = False)
    if not proj:
        return

    # We put generated LUCI configs under proj.props.config_dir, see set_config.
    # Declare it is a project config set, so it is sent to LUCI config for
    # validation.
    ctx.declare_config_set("projects/%s" % proj.props.name, proj.props.config_dir)

    # Find all PROJECT_CONFIGS_READER role entries.
    access = []
    for a in filter_acls(get_project_acls(), [acl.PROJECT_CONFIGS_READER]):
        if a.user:
            access.append("user:" + a.user)
        elif a.group:
            access.append("group:" + a.group)
        elif a.project:
            access.append("project:" + a.project)
        else:
            fail("impossible")

    # Path to the generated LUCI config directory relative to the main package
    # root.
    config_dir = strutil.join_path(
        __native__.get_meta("config_dir"),
        proj.props.config_dir,
        allow_dots = True,
    )

    emit_metadata = not __native__.running_tests and not proj.props.omit_lucicfg_metadata

    set_config(ctx, "project.cfg", config_pb.ProjectCfg(
        name = proj.props.name,
        access = access,
        lucicfg = config_pb.GeneratorMetadata(
            version = "%d.%d.%d" % lucicfg.version(),
            config_dir = config_dir,
            package_dir = __native__.package_dir(config_dir),
            entry_point = __native__.entry_point,
            vars = __native__.var_flags,
            experiments = __native__.list_enabled_experiments(),
        ) if emit_metadata else None,
    ))

################################################################################
## realm.cfg.

def realms_cfg(proj):
    """Returns either `realms.cfg` or `realms-dev.cfg`."""
    return "realms-dev.cfg" if proj.props.dev else "realms.cfg"

def gen_realms_cfg(ctx):
    """Generates realms.cfg.

    Args:
      ctx: the generator context.
    """
    proj = get_project(required = False)
    if proj:
        set_config(
            ctx = ctx,
            path = realms_cfg(proj),
            cfg = realms.generate_realms_cfg(realms.default_impl),
        )

################################################################################
## logdog.cfg.

def gen_logdog_cfg(ctx):
    """Generates logdog.cfg.

    Args:
      ctx: the generator context.
    """
    opts = graph.node(keys.logdog())
    if not opts:
        return

    # Note that acl.LOGDOG_* are declared as groups_only=True roles, so .group
    # is guaranteed to be set here.
    readers = []
    writers = []
    for a in get_project_acls():
        if a.role == acl.LOGDOG_READER:
            readers.append(a.group)
        elif a.role == acl.LOGDOG_WRITER:
            writers.append(a.group)

    cl_cfg = None
    if opts.props.cloud_logging_project:
        cl_cfg = logdog_cloud_logging_pb.CloudLoggingConfig(
            destination = opts.props.cloud_logging_project,
        )

    logdog = get_service("logdog", "defining LogDog options")
    set_config(ctx, logdog.cfg_file, logdog_pb.ProjectConfig(
        reader_auth_groups = readers,
        writer_auth_groups = writers,
        archive_gs_bucket = opts.props.gs_bucket,
        cloud_logging_config = cl_cfg,
    ))

################################################################################
## buildbucket.cfg.

# acl.role => buildbucket_pb.Acl.Role.
_buildbucket_roles = {
    acl.BUILDBUCKET_READER: buildbucket_pb.Acl.READER,
    acl.BUILDBUCKET_TRIGGERER: buildbucket_pb.Acl.SCHEDULER,
    acl.BUILDBUCKET_OWNER: buildbucket_pb.Acl.WRITER,
}

def gen_buildbucket_cfg(ctx):
    """Generates buildbucket.cfg.

    Args:
      ctx: the generator context.
    """

    # TODO(randymaldoando): crbug/399576 - move builders up a level and
    # replace swarming in bucket proto.
    buckets = get_buckets()
    if not buckets:
        return

    cfg = buildbucket_pb.BuildbucketCfg()
    buildbucket = get_service("buildbucket", "defining buckets")
    set_config(ctx, buildbucket.cfg_file, cfg)
    _buildbucket_check_connections()

    shadow_bucket_constraints = _buildbucket_shadow_bucket_constraints(buckets)
    for bucket in buckets:
        swarming = _buildbucket_builders(bucket)
        dynamic_builder_template = _buildbucket_dynamic_builder_template(bucket)
        if dynamic_builder_template != None and swarming != None:
            error("dynamic bucket \"%s\" must not have pre-defined builders" % bucket.props.name, trace = bucket.trace)

        constraints = _buildbucket_constraints(bucket)
        if shadow_bucket_constraints and shadow_bucket_constraints.get(bucket.props.name):
            if not constraints:
                constraints = buildbucket_pb.Bucket.Constraints()
            additional_constraints = shadow_bucket_constraints[bucket.props.name]
            constraints.pools.extend(additional_constraints.pools)
            constraints.pools = sorted(set(constraints.pools))
            constraints.service_accounts.extend(additional_constraints.service_accounts)
            constraints.service_accounts = sorted(set(constraints.service_accounts))

        cfg.buckets.append(buildbucket_pb.Bucket(
            name = bucket.props.name,
            acls = _buildbucket_acls(get_bucket_acls(bucket)),
            swarming = swarming,
            shadow = _buildbucket_shadow(bucket),
            constraints = constraints,
            dynamic_builder_template = dynamic_builder_template,
        ))

    if shadow_bucket_constraints:
        _gen_shadow_service_account_bindings(
            ctx.output[output_path(realms_cfg(get_project()))],
            shadow_bucket_constraints,
        )

    topics = [
        buildbucket_pb.BuildbucketCfg.Topic(
            name = t.props.name,
            compression = t.props.compression,
        )
        for t in get_bb_notification_topics()
    ]
    if topics:
        cfg.common_config = buildbucket_pb.BuildbucketCfg.CommonConfig(
            builds_notification_topics = topics,
        )

def _buildbucket_check_connections():
    """Ensures all luci.bucket_constraints(...) are connected to one and only one luci.bucket(...)."""
    root = keys.bucket_constraints_root()
    for e in graph.children(root):
        buckets = [p for p in graph.parents(e.key) if p.key != root]
        if len(buckets) == 0:
            error("%s is not added to any bucket, either remove or comment it out" % e, trace = e.trace)
        elif len(buckets) > 1:
            error(
                "%s is added to multiple buckets: %s" %
                (e, ", ".join([str(v) for v in buckets])),
                trace = e.trace,
            )

def _buildbucket_acls(elementary):
    """[acl.elementary] => filtered [buildbucket_pb.Acl]."""
    if not _legacy_acls():
        return []
    return [
        buildbucket_pb.Acl(
            role = _buildbucket_roles[a.role],
            group = a.group,
            identity = None if a.group else _buildbucket_identity(a),
        )
        for a in filter_acls(elementary, _buildbucket_roles.keys())
    ]

def _buildbucket_identity(a):
    """acl.elementary => identity string for buildbucket_pb.Acl."""
    if a.user:
        return "user:" + a.user
    if a.project:
        return "project:" + a.project
    fail("impossible")

def _buildbucket_shadow_bucket_constraints(buckets):
    """a list of luci.bucket(...) nodes => a dict of bucket name to constraints."""
    shadow_bucket_constraints = {}
    for bucket in buckets:
        service_accounts = []
        pools = []
        for node in graph.children(bucket.key, kinds.BUILDER):
            if node.props.shadow_service_account:
                service_accounts.append(node.props.shadow_service_account)
            if node.props.shadow_pool:
                pools.append(node.props.shadow_pool)

        if len(service_accounts) == 0 and len(pools) == 0:
            continue
        shadow = _buildbucket_shadow(bucket)
        if not shadow:
            error(
                "builders in bucket %s set shadow_service_account or shadow_pool, but the bucket does not have a shadow bucket" %
                bucket.props.name,
                trace = bucket.trace,
            )
            return None
        constraints = shadow_bucket_constraints.setdefault(shadow, struct(
            service_accounts = [],
            pools = [],
        ))
        constraints.service_accounts.extend(service_accounts)
        constraints.pools.extend(pools)
    return shadow_bucket_constraints

def _buildbucket_builder(node, def_swarming_host):
    if node.key.kind != kinds.BUILDER and node.key.kind != kinds.DYNAMIC_BUILDER_TEMPLATE:
        fail("impossible: can only generate builder config for a builder or dynamic_builder_template")
    exe, recipe, properties, experiments = _handle_executable(node)
    combined_experiments = dict(node.props.experiments)
    combined_experiments.update(experiments)
    task_template_canary_percentage = None
    if _use_experiment_for_task_template_canary_percentage.is_enabled():
        if node.props.task_template_canary_percentage:
            combined_experiments["luci.buildbucket.canary_software"] = node.props.task_template_canary_percentage
    else:
        task_template_canary_percentage = optional_UInt32Value(
            node.props.task_template_canary_percentage,
        )
    bldr_config = buildbucket_pb.BuilderConfig(
        name = node.props.name,
        description_html = node.props.description_html,
        exe = exe,
        recipe = recipe,
        properties = properties,
        allowed_property_overrides = sorted(node.props.allowed_property_overrides),
        service_account = node.props.service_account,
        caches = _buildbucket_caches(node.props.caches),
        execution_timeout_secs = optional_sec(node.props.execution_timeout),
        grace_period = optional_duration_pb(node.props.grace_period),
        heartbeat_timeout_secs = optional_sec(node.props.heartbeat_timeout),
        max_concurrent_builds = node.props.max_concurrent_builds,
        dimensions = _buildbucket_dimensions(node.props.dimensions),
        priority = node.props.priority,
        expiration_secs = optional_sec(node.props.expiration_timeout),
        wait_for_capacity = _buildbucket_trinary(node.props.wait_for_capacity),
        retriable = _buildbucket_trinary(node.props.retriable),
        build_numbers = _buildbucket_toggle(node.props.build_numbers),
        experimental = _buildbucket_toggle(node.props.experimental),
        experiments = combined_experiments,
        task_template_canary_percentage = task_template_canary_percentage,
        resultdb = node.props.resultdb,
        contact_team_email = node.props.contact_team_email,
        custom_metric_definitions = _buildbucket_custom_metrics(node.props.custom_metrics),
    )
    if node.props.backend != None:
        backend = graph.node(node.props.backend)
        bldr_config.backend = buildbucket_pb.BuilderConfig.Backend(
            target = backend.props.target,
            config_json = backend.props.config,
        )
    if node.props.backend_alt != None:
        backend_alt = graph.node(node.props.backend_alt)
        bldr_config.backend_alt = buildbucket_pb.BuilderConfig.Backend(
            target = backend_alt.props.target,
            config_json = backend_alt.props.config,
        )

    swarming_host = node.props.swarming_host
    if node.props.backend == None and node.props.backend_alt == None and not swarming_host:
        if not def_swarming_host:
            def_swarming_host = get_service("swarming", "defining builders").host
        swarming_host = def_swarming_host
    if swarming_host:
        bldr_config.swarming_host = swarming_host
        bldr_config.swarming_tags = node.props.swarming_tags

    if node.props.shadow_service_account or node.props.shadow_pool or node.props.shadow_properties or node.props.shadow_dimensions:
        bldr_config.shadow_builder_adjustments = buildbucket_pb.BuilderConfig.ShadowBuilderAdjustments(
            service_account = node.props.shadow_service_account,
            pool = node.props.shadow_pool,
            properties = to_json(node.props.shadow_properties) if node.props.shadow_properties else None,
            dimensions = _buildbucket_dimensions(node.props.shadow_dimensions, allow_none = True),
        )

    return bldr_config, def_swarming_host

def _buildbucket_builders(bucket):
    """luci.bucket(...) node => buildbucket_pb.Swarming or None."""
    def_swarming_host = None
    builders = []
    for node in graph.children(bucket.key, kinds.BUILDER):
        bldr_config, def_swarming_host = _buildbucket_builder(node, def_swarming_host)
        builders.append(bldr_config)
    return buildbucket_pb.Swarming(builders = builders) if builders else None

def _handle_executable(node):
    """Handle a builder node's executable node.

    Builder node =>
      buildbucket_pb.BuilderConfig.Recipe | common_pb.Executable,
      buildbucket_pb.BuilderConfig.Properties, buildbucket_pb.BuilderConfig.Experiments

    This function produces either a Recipe or Executable definition depending on
    whether executable.props.recipe was set. luci.recipe(...) will always set
    executable.props.recipe.

    If we're handling a recipe, set properties_j in the Recipe definition.
    If we're handling a normal executable, return Properties to be assigned to
    Builder.Properties.

    When we are ready to move config output entirely from recipes to their
    exe equivalents, we can stop producing Recipe definitions here.
    """
    if node.key.kind != kinds.BUILDER and node.key.kind != kinds.DYNAMIC_BUILDER_TEMPLATE:
        fail("impossible: can only handle executable for a builder or dynamic_builder_template")

    executables = graph.children(node.key, kinds.EXECUTABLE)
    if len(executables) != 1:
        if node.key.kind == kinds.BUILDER:
            fail("impossible: the builder should have a reference to an executable")
        else:
            # a dynamic_builder_template without executable is allowed.
            properties = to_json(node.props.properties) if node.props.properties else None
            return None, None, properties, {}
    experiments = {}
    executable = executables[0]
    if not executable.props.cmd and executable.props.recipe:
        # old kitchen way
        recipe_def = buildbucket_pb.BuilderConfig.Recipe(
            name = executable.props.recipe,
            cipd_package = executable.props.cipd_package,
            cipd_version = executable.props.cipd_version,
            properties_j = sorted([
                "%s:%s" % (k, to_json(v))
                for k, v in node.props.properties.items()
            ]),
        )
        executable_def = None
        properties = None
    else:
        executable_def = common_pb.Executable(
            cipd_package = executable.props.cipd_package,
            cipd_version = executable.props.cipd_version,
            cmd = executable.props.cmd,
        )
        if executable.props.wrapper:
            executable_def.wrapper = executable.props.wrapper
        recipe_def = None
        props_dict = node.props.properties
        if executable.props.recipe:
            props_dict = dict(props_dict)
            props_dict["recipe"] = executable.props.recipe
        properties = to_json(props_dict)
        if executable.props.recipes_py3:
            experiments["luci.recipes.use_python3"] = 100
    return executable_def, recipe_def, properties, experiments

def _buildbucket_caches(caches):
    """[swarming.cache] => [buildbucket_pb.BuilderConfig.CacheEntry]."""
    out = []
    for c in caches:
        out.append(buildbucket_pb.BuilderConfig.CacheEntry(
            name = c.name,
            path = c.path,
            wait_for_warm_cache_secs = optional_sec(c.wait_for_warm_cache),
        ))
    return sorted(out, key = lambda x: x.name)

def _buildbucket_dimensions(dims, allow_none = False):
    """{str: [swarming.dimension]} => [str] for 'dimensions' field."""
    out = []
    for key in sorted(dims):
        if allow_none and dims[key] == None:
            out.append("%s:" % key)
            continue
        for d in dims[key]:
            if d.expiration == None:
                out.append("%s:%s" % (key, d.value))
            else:
                out.append("%d:%s:%s" % (d.expiration // time.second, key, d.value))
    return out

def _buildbucket_trinary(val):
    """Bool|None => common_pb.Trinary."""
    if val == None:
        return common_pb.UNSET
    return common_pb.YES if val else common_pb.NO

def _buildbucket_toggle(val):
    """Bool|None => buildbucket_pb.Toggle."""
    if val == None:
        return buildbucket_pb.UNSET
    return buildbucket_pb.YES if val else buildbucket_pb.NO

def _buildbucket_shadow(bucket):
    """luci.bucket(...) node => buildbucket_pb.Shadow or None."""
    shadow = graph.node(keys.shadow_of(bucket.key))
    if shadow:
        return shadow.props.shadow
    return None

def _buildbucket_constraints(bucket):
    """luci.bucket(...) node => buildbucket_pb.Bucket.Constraints or None."""
    pools = set()
    service_accounts = set()
    for node in graph.children(bucket.key, kinds.BUCKET_CONSTRAINTS):
        pools |= set(node.props.pools)
        service_accounts |= set(node.props.service_accounts)
    pools = sorted(pools)
    service_accounts = sorted(service_accounts)
    if len(pools) == 0 and len(service_accounts) == 0:
        return None
    return buildbucket_pb.Bucket.Constraints(pools = pools, service_accounts = service_accounts)

def _gen_shadow_service_account_bindings(realms_cfg, shadow_bucket_constraints):
    """Mutates realms.cfg by adding `role/buildbucket.builderServiceAccount` bindings.

    This function is to add builders' shadow_service_accounts to their shadow
    buckets as builder service accounts.

    Args:
      realms_cfg: realms_pb.RealmsCfg to mutate.
      shadow_bucket_constraints: a dict of bucket name to constraints
    """
    for bucket, constraints in shadow_bucket_constraints.items():
        principals = sorted(set(["user:" + account for account in constraints.service_accounts]))
        if len(principals) == 0:
            continue
        realms.append_binding_pb(realms_cfg, keys.realm(bucket).id, realms_pb.Binding(
            role = "role/buildbucket.builderServiceAccount",
            principals = principals,
        ))

def _buildbucket_custom_metrics(custom_metrics):
    """[buildbucket.custom_metric] => [buildbucket_pb.CustomMetricDefinition]."""
    out = []
    for cm in custom_metrics:
        out.append(buildbucket_pb.CustomMetricDefinition(
            name = cm.name,
            predicates = cm.predicates,
            extra_fields = cm.extra_fields,
        ))
    return sorted(out, key = lambda x: x.name)

################################################################################
## scheduler.cfg.

# acl.role => scheduler_pb.Acl.Role.
_scheduler_roles = {
    acl.SCHEDULER_READER: scheduler_pb.Acl.READER,
    acl.SCHEDULER_TRIGGERER: scheduler_pb.Acl.TRIGGERER,
    acl.SCHEDULER_OWNER: scheduler_pb.Acl.OWNER,
}

# Enables generation of shorter BuildbucketTask protos and conditional bindings.
_scheduler_use_bb_v2 = experiments.register("crbug.com/1182002")

def gen_scheduler_cfg(ctx):
    """Generates scheduler.cfg.

    Args:
      ctx: the generator context.
    """
    buckets = get_buckets()
    if not buckets:
        return

    # Discover who triggers who, validate there are no ambiguities in 'triggers'
    # and 'triggered_by' relations (triggerer.targets reports them as errors).

    pollers = {}  # GITILES_POLLER node -> list of BUILDER nodes it triggers.
    builders = {}  # BUILDER node -> list GITILES_POLLER|BUILDER that trigger it (if any).

    def want_scheduler_job_for(builder):
        if builder not in builders:
            builders[builder] = []

    def add_triggered_by(builder, triggered_by):
        want_scheduler_job_for(builder)
        builders[builder].append(triggered_by)

    for bucket in buckets:
        for poller in graph.children(bucket.key, kinds.GITILES_POLLER):
            # Note: the list of targets may be empty. We still want to define a
            # poller, so add the entry to the dict regardless. This may be
            # useful to confirm a poller works before making it trigger
            # anything.
            pollers[poller] = triggerer.targets(poller)
            for target in pollers[poller]:
                add_triggered_by(target, triggered_by = poller)

        for builder in graph.children(bucket.key, kinds.BUILDER):
            targets = triggerer.targets(builder)
            if targets and not builder.props.service_account:
                error(
                    "%s needs service_account set, it triggers other builders: %s" %
                    (builder, ", ".join([str(t) for t in targets])),
                    trace = builder.trace,
                )
            else:
                for target in targets:
                    add_triggered_by(target, triggered_by = builder)

            # If using a cron schedule or a custom triggering policy, need to
            # setup a Job entity for this builder.
            if builder.props.schedule or builder.props.triggering_policy:
                want_scheduler_job_for(builder)

    # List of BUILDER and GITILES_POLLER nodes we need an entity in the
    # scheduler config for.
    entities = pollers.keys() + builders.keys()

    # The scheduler service is not used at all? Don't require its hostname then.
    if not entities:
        return

    scheduler = get_service("scheduler", "using triggering or pollers")
    buildbucket = get_service("buildbucket", "using triggering")
    project = get_project()

    cfg = scheduler_pb.ProjectConfig()
    set_config(ctx, scheduler.cfg_file, cfg)

    if _legacy_acls():
        # Generate per-bucket ACL sets based on bucket-level permissions. Skip
        # buckets that aren't used to avoid polluting configs with unnecessary
        # entries.
        for bucket in get_buckets_of(entities):
            cfg.acl_sets.append(scheduler_pb.AclSet(
                name = bucket.props.name,
                acls = _scheduler_acls(get_bucket_acls(bucket)),
            ))

    # We prefer to use bucket-less names in the scheduler configs, so that IDs
    # that show up in logs and in the debug UI match what is used in the
    # starlark config. On conflicts, append the bucket name as suffix to
    # disambiguate.
    #
    # TODO(vadimsh): Revisit this if LUCI Scheduler starts supporting buckets
    # directly. Right now each project has a single flat namespace of job IDs
    # and all existing configs use 'scheduler job name == builder name'.
    # Artificially injecting bucket names into all job IDs will complicate the
    # migration to lucicfg by obscuring diffs between existing configs and new
    # generated configs.
    node_to_id = _scheduler_disambiguate_ids(entities)

    # Add Trigger entry for each gitiles poller. Sort according to final IDs.
    for poller, targets in pollers.items():
        cfg.trigger.append(scheduler_pb.Trigger(
            id = node_to_id[poller],
            realm = poller.props.realm,
            acl_sets = [poller.props.bucket] if _legacy_acls() else [],
            triggers = [node_to_id[b] for b in targets],
            schedule = poller.props.schedule,
            gitiles = scheduler_pb.GitilesTask(
                repo = poller.props.repo,
                refs = ["regexp:" + r for r in poller.props.refs],
                path_regexps = poller.props.path_regexps,
                path_regexps_exclude = poller.props.path_regexps_exclude,
            ),
        ))
    cfg.trigger = sorted(cfg.trigger, key = lambda x: x.id)

    # Add Job entry for each triggered or cron builder. Grant all triggering
    # builders (if any) TRIGGERER role. Sort according to final IDs.
    for builder, triggered_by in builders.items():
        cfg.job.append(scheduler_pb.Job(
            id = node_to_id[builder],
            realm = builder.props.realm,
            acl_sets = [builder.props.bucket] if _legacy_acls() else [],
            acls = _scheduler_acls(aclimpl.normalize_acls([
                acl.entry(acl.SCHEDULER_TRIGGERER, users = t.props.service_account)
                for t in triggered_by
                if t.key.kind == kinds.BUILDER
            ])),
            schedule = builder.props.schedule,
            triggering_policy = builder.props.triggering_policy,
            buildbucket = _scheduler_task(builder, buildbucket, project.props.name),
        ))
    cfg.job = sorted(cfg.job, key = lambda x: x.id)

    # Add conditional "role/scheduler.triggerer" bindings that allow builders to
    # trigger jobs.
    if _scheduler_realms_configs():
        _gen_scheduler_bindings(
            ctx.output[output_path(realms_cfg(project))],
            builders,
            node_to_id,
        )

def _scheduler_realms_configs():
    """True to generate realms-only Scheduler configs."""
    return _scheduler_use_bb_v2.is_enabled() or not _legacy_acls()

def _scheduler_disambiguate_ids(nodes):
    """[graph.node] => dict{node: name to use for it in scheduler.cfg}."""

    # Build dict: name -> [nodes that have it].
    claims = {}
    for n in nodes:
        nm = n.props.name
        if nm not in claims:
            claims[nm] = []
        claims[nm].append(n)

    names = {}  # node -> name
    used = {}  # name -> node, reverse of 'names'

    # use_name deals with the second layer of ambiguities: when our generated
    # '<name>-<bucket>' name happened to clash with some other '<name>'. This
    # should be super rare. Lack of 'while' in starlark makes it difficult to
    # handle such collisions automatically, so we just give up and ask the user
    # to pick some other names.
    def use_name(name, node):
        if name in used:
            fail(
                (
                    "%s and %s cause ambiguities in the scheduler config file, " +
                    "pick names that don't start with a bucket name"
                ) % (node, used[name]),
                trace = node.trace,
            )
        names[node] = name
        used[name] = node

    for nm, candidates in claims.items():
        if len(candidates) == 1:
            use_name(nm, candidates[0])
        else:
            for node in candidates:
                use_name("%s-%s" % (node.props.bucket, nm), node)

    return names

def _gen_scheduler_bindings(realms_cfg, builders, node_to_id):
    """Mutates realms.cfg by adding `role/scheduler.triggerer` bindings.

    Args:
      realms_cfg: realms_pb.RealmsCfg to mutate.
      builders: BUILDER node -> list GITILES_POLLER|BUILDER that trigger it.
      node_to_id: dict{BUILDER node: name to use for it in scheduler.cfg}.
    """

    # (target realm, triggering service account) => [triggered job ID].
    per_realm_per_account = {}
    for builder, triggered_by in builders.items():
        job_id = node_to_id[builder]
        job_realm = builder.props.realm
        for t in triggered_by:
            if t.key.kind == kinds.BUILDER and t.props.service_account:
                key = (job_realm, t.props.service_account)
                per_realm_per_account.setdefault(key, []).append(job_id)

    # Append corresponding bindings to realms.cfg.
    for realm, account in sorted(per_realm_per_account):
        jobs = sorted(set(per_realm_per_account[(realm, account)]))
        realms.append_binding_pb(realms_cfg, realm, realms_pb.Binding(
            role = "role/scheduler.triggerer",
            principals = ["user:" + account],
            conditions = [
                realms_pb.Condition(
                    restrict = realms_pb.Condition.AttributeRestriction(
                        attribute = "scheduler.job.name",
                        values = jobs,
                    ),
                ),
            ],
        ))

def _scheduler_acls(elementary):
    """[acl.elementary] => filtered [scheduler_pb.Acl]."""
    if not _legacy_acls():
        return []
    return [
        scheduler_pb.Acl(
            role = _scheduler_roles[a.role],
            granted_to = _scheduler_identity(a),
        )
        for a in filter_acls(elementary, _scheduler_roles.keys())
    ]

def _scheduler_identity(a):
    """acl.elementary => identity string for scheduler_pb.Acl."""
    if a.user:
        return a.user
    if a.group:
        return "group:" + a.group
    if a.project:
        return "project:" + a.project
    fail("impossible")

def _scheduler_task(builder, buildbucket, project_name):
    """Produces scheduler_pb.BuildbucketTask for a scheduler job."""
    if not _scheduler_realms_configs():
        bucket = legacy_bucket_name(builder.props.bucket, project_name)
    else:
        bucket = builder.props.bucket
    return scheduler_pb.BuildbucketTask(
        server = buildbucket.host,
        bucket = bucket,
        builder = builder.props.name,
    )

################################################################################
## milo.cfg.

def gen_milo_cfg(ctx):
    """Generates milo.cfg.

    Args:
      ctx: the generator context.
    """
    _milo_check_connections()

    # Note: luci.milo(...) node is optional.
    milo_node = graph.node(keys.milo())
    opts = struct(
        logo = milo_node.props.logo if milo_node else None,
        favicon = milo_node.props.favicon if milo_node else None,
        bug_url_template = milo_node.props.bug_url_template if milo_node else None,
    )

    # Keep the order of views as they were defined, for Milo's list of consoles.
    views = graph.children(keys.project(), kinds.MILO_VIEW, order_by = graph.DEFINITION_ORDER)
    if not views and not milo_node:
        return

    milo = get_service("milo", "using views or setting Milo config")
    project_name = get_project().props.name

    set_config(ctx, milo.cfg_file, milo_pb.Project(
        bug_url_template = opts.bug_url_template,
        logo_url = opts.logo,
        consoles = [
            _milo_console_pb(view, opts, project_name)
            for view in views
        ],
    ))

def _milo_check_connections():
    """Ensures all *_view_entry are connected to one and only one *_view."""
    root = keys.milo_entries_root()
    for e in graph.children(root):
        views = [p for p in graph.parents(e.key) if p.key != root]
        if len(views) == 0:
            error("%s is not added to any view, either remove or comment it out" % e, trace = e.trace)
        elif len(views) > 1:
            error(
                "%s is added to multiple views: %s" %
                (e, ", ".join([str(v) for v in views])),
                trace = e.trace,
            )

def _milo_console_pb(view, opts, project_name):
    """Given MILO_VIEW node returns milo_pb.Console."""
    ch = graph.children(view.key)
    if len(ch) != 1:
        fail("impossible: %s" % (ch,))
    view = ch[0]
    if view.key.kind == kinds.LIST_VIEW:
        return _milo_list_view(view, opts, project_name)
    if view.key.kind == kinds.CONSOLE_VIEW:
        return _milo_console_view(view, opts, project_name)
    if view.key.kind == kinds.EXTERNAL_CONSOLE_VIEW:
        return _milo_external_console_view(view)
    fail("impossible: %s" % (view,))

def _milo_external_console_view(view):
    """Given an EXTERNAL_CONSOLE_VIEW node returns milo_pb.Console."""
    return milo_pb.Console(
        id = view.props.name,
        name = view.props.title,
        external_project = view.props.external_project,
        external_id = view.props.external_id,
    )

def _milo_list_view(view, opts, project_name):
    """Given a LIST_VIEW node produces milo_pb.Console."""
    builders = []
    seen = {}
    for e in graph.children(view.key, order_by = graph.DEFINITION_ORDER):
        pb = _milo_builder_pb(e, view, project_name, seen)
        if pb:
            builders.append(pb)
    return milo_pb.Console(
        id = view.props.name,
        name = view.props.title,
        favicon_url = view.props.favicon or opts.favicon,
        builder_view_only = True,
        builders = builders,
    )

def _milo_console_view(view, opts, project_name):
    """Given a CONSOLE_VIEW node produces milo_pb.Console."""
    builders = []
    seen = {}
    for e in graph.children(view.key, order_by = graph.DEFINITION_ORDER):
        pb = _milo_builder_pb(e, view, project_name, seen)
        if pb:
            pb.short_name = e.props.short_name
            pb.category = e.props.category
            builders.append(pb)

    return milo_pb.Console(
        id = view.props.name,
        name = view.props.title,
        header = view.props.header,
        repo_url = view.props.repo,
        refs = ["regexp:" + r for r in view.props.refs],
        exclude_ref = view.props.exclude_ref,

        # TODO(hinoka,iannucci): crbug/832893 - Support custom manifest names,
        # such as 'UNPATCHED' / 'PATCHED'.
        manifest_name = "REVISION",
        include_experimental_builds = view.props.include_experimental_builds,
        favicon_url = view.props.favicon or opts.favicon,
        builders = builders,
        default_commit_limit = view.props.default_commit_limit,
        default_expand = view.props.default_expand,
    )

def _milo_builder_pb(entry, view, project_name, seen):
    """Returns milo_pb.Builder given *_VIEW_ENTRY node.

    Args:
      entry: a *_VIEW_ENTRY node.
      view: a parent *_VIEW node (for error messages).
      project_name: LUCI project name, to expand short bucket names into
        luci.<project>.<bucket> ones.
      seen: a dict {BUILDER key -> *_VIEW_ENTRY node that added it} with
        already added builders, to detect dups. Mutated.

    Returns:
      milo_pb.Builder or None on errors.
    """
    builder_pb = milo_pb.Builder()

    # Note: this is a one-item list for regular entries per *_view_entry
    # implementation.
    refs = graph.children(entry.key, kinds.BUILDER_REF)
    if len(refs) != 1:
        fail("impossible result %s" % (refs,))

    # Grab BUILDER node and ensure it hasn't be added to this view yet.
    builder = builder_ref.follow(refs[0], context_node = entry)
    if builder.key in seen:
        error(
            "builder %s was already added to %s, previous declaration:\n%s" %
            (builder, view, seen[builder.key].trace),
            trace = entry.trace,
        )
        return None
    seen[builder.key] = entry

    builder_pb.name = "buildbucket/%s/%s" % (
        legacy_bucket_name(
            builder.props.bucket,
            builder.props.project or project_name,
        ),
        builder.props.name,
    )
    return builder_pb

################################################################################
## commit-queue.cfg.

def gen_cq_cfg(ctx):
    """Generates commit-queue.cfg.

    Args:
      ctx: the generator context.
    """
    _cq_check_connections()

    cq_groups = graph.children(keys.project(), kind = kinds.CQ_GROUP)
    if not cq_groups:
        return

    # Note: commit-queue.cfg without any ConfigGroup is forbidden by CQ, but we
    # still allow to specify luci.cq(...) in this case (it is just ignored).
    cq_node = graph.node(keys.cq())
    cfg = cq_pb.Config(
        cq_status_host = cq_node.props.status_host if cq_node else None,
        draining_start_time = cq_node.props.draining_start_time if cq_node else None,
        honor_gerrit_linked_accounts = (
            cq_node.props.honor_gerrit_linked_accounts if cq_node else None
        ),
    )
    set_config(ctx, "commit-queue.cfg", cfg)

    if cq_node and cq_node.props.submit_max_burst:
        cfg.submit_options = cq_pb.SubmitOptions(
            max_burst = cq_node.props.submit_max_burst,
            burst_delay = duration_pb.Duration(
                seconds = cq_node.props.submit_burst_delay // time.second,
            ),
        )

    # Each luci.cq_group(...) results in a separate cq_pb.ConfigGroup.
    cfg.config_groups = [
        _cq_config_group(g, get_project())
        for g in cq_groups
    ]

def _cq_check_connections():
    """Ensures all cq_tryjob_verifier are connected to one and only one group."""
    for v in graph.children(keys.cq_verifiers_root()):
        groups = graph.parents(v.key, kind = kinds.CQ_GROUP)
        if len(groups) == 0:
            error("%s is not added to any cq_group, either remove or comment it out" % v, trace = v.trace)
        elif len(groups) > 1:
            error(
                "%s is added to multiple cq_groups: %s" %
                (v, ", ".join([str(g) for g in groups])),
                trace = v.trace,
            )

def _cq_config_group(cq_group, project):
    """Given a cq_group node returns cq_pb.ConfigGroup."""
    acls = aclimpl.normalize_acls(cq_group.props.acls + project.props.acls)
    gerrit_cq_ability = cq_pb.Verifiers.GerritCQAbility(
        committer_list = [a.group for a in filter_acls(acls, [acl.CQ_COMMITTER])],
        dry_run_access_list = [a.group for a in filter_acls(acls, [acl.CQ_DRY_RUNNER])],
        new_patchset_run_access_list = [a.group for a in filter_acls(acls, [acl.CQ_NEW_PATCHSET_RUN_TRIGGERER])],
        allow_submit_with_open_deps = cq_group.props.allow_submit_with_open_deps,
        allow_owner_if_submittable = cq_group.props.allow_owner_if_submittable,
        trust_dry_runner_deps = cq_group.props.trust_dry_runner_deps,
        allow_non_owner_dry_runner = cq_group.props.allow_non_owner_dry_runner,
    )
    if not gerrit_cq_ability.committer_list:
        error("at least one CQ_COMMITTER acl.entry must be specified (either here or in luci.project)", trace = cq_group.trace)

    tree_status = None
    if cq_group.props.tree_status_name:
        tree_status = cq_pb.Verifiers.TreeStatus(tree_name = cq_group.props.tree_status_name)
    elif cq_group.props.tree_status_host:
        tree_status = cq_pb.Verifiers.TreeStatus(url = "https://" + cq_group.props.tree_status_host)

    # Note: CQ_TRYJOB_VERIFIER nodes are by default lexicographically sorted
    # according to auto-generated unique keys (that do not show up in the
    # output). We prefer to sort by user-visible 'name' instead.
    seen = {}  # _cq_builder_name(...) -> verifier node that added it
    tryjob = cq_pb.Verifiers.Tryjob(
        retry_config = _cq_retry_config(cq_group.props.retry_config),
        builders = sorted([
            _cq_tryjob_builder(v, cq_group, project, seen)
            for v in graph.children(cq_group.key, kind = kinds.CQ_TRYJOB_VERIFIER)
        ], key = lambda b: b.name),
    )

    group_by_gob_host = {}
    for w in cq_group.props.watch:
        if w.__kind != "gob":
            fail("only Gerrit repos are supported")
        group_by_gob_host.setdefault(w.__gob_host, []).append(w)

    user_limits = [_cq_user_limit(q) for q in cq_group.props.user_limits]
    user_limit_default = cq_group.props.user_limit_default
    if user_limit_default != None:
        user_limit_default = _cq_user_limit(user_limit_default)
    post_actions = cq_group.props.post_actions
    if post_actions != None:
        post_actions = [_cq_post_action(pa) for pa in post_actions]
    tryjob_experiments = cq_group.props.tryjob_experiments
    if tryjob_experiments != None:
        tryjob_experiments = [
            _cq_tryjob_experiment(te)
            for te in tryjob_experiments
        ]

    return cq_pb.ConfigGroup(
        name = cq_group.key.id,
        gerrit = [
            cq_pb.ConfigGroup.Gerrit(
                url = "https://%s-review.googlesource.com" % host,
                projects = [
                    cq_pb.ConfigGroup.Gerrit.Project(
                        name = w.__gob_proj,
                        ref_regexp = w.__refs,
                        ref_regexp_exclude = w.__refs_exclude,
                    )
                    for w in watches
                ],
            )
            for host, watches in group_by_gob_host.items()
        ],
        verifiers = cq_pb.Verifiers(
            gerrit_cq_ability = gerrit_cq_ability,
            tree_status = tree_status,
            tryjob = tryjob if tryjob.builders else None,
        ),
        additional_modes = [
            _cq_run_mode(m)
            for m in cq_group.props.additional_modes
        ] if cq_group.props.additional_modes else None,
        user_limits = user_limits,
        user_limit_default = user_limit_default,
        post_actions = post_actions,
        tryjob_experiments = tryjob_experiments,
    )

def _cq_retry_config(retry_config):
    """cq._retry_config(...) => cq_pb.Verifiers.Tryjob.RetryConfig."""
    if not retry_config:
        return None
    return cq_pb.Verifiers.Tryjob.RetryConfig(
        single_quota = retry_config.single_quota,
        global_quota = retry_config.global_quota,
        failure_weight = retry_config.failure_weight,
        transient_failure_weight = retry_config.transient_failure_weight,
        timeout_weight = retry_config.timeout_weight,
    )

def _cq_post_action(post_action):
    """Converts a post action to cq_pb.ConfigGroup.PostAction."""
    ret = cq_pb.ConfigGroup.PostAction(
        name = post_action.name,
        conditions = [
            cq_pb.ConfigGroup.PostAction.TriggeringCondition(
                mode = cond.mode,
                statuses = cond.statuses,
            )
            for cond in post_action.conditions
        ],
    )
    if post_action.vote_gerrit_labels != None:
        ret.vote_gerrit_labels = cq_pb.ConfigGroup.PostAction.VoteGerritLabels(
            votes = [
                cq_pb.ConfigGroup.PostAction.VoteGerritLabels.Vote(
                    name = k,
                    value = v,
                )
                for k, v in post_action.vote_gerrit_labels.items()
            ],
        )

    return ret

def _cq_tryjob_experiment(experiment):
    """Converts a tryjob experiment to cq_pb.ConfigGroup.TryjobExperiment."""
    ret = cq_pb.ConfigGroup.TryjobExperiment(
        name = experiment.name,
    )
    if experiment.owner_group_allowlist:
        ret.condition = cq_pb.ConfigGroup.TryjobExperiment.Condition(
            owner_group_allowlist = experiment.owner_group_allowlist,
        )
    return ret

def _cq_run_mode(run_mode):
    """cq._run_mode(...) => cq_pb.Mode."""
    if not run_mode:
        return None
    return cq_pb.Mode(
        name = run_mode.name,
        cq_label_value = run_mode.cq_label_value,
        triggering_label = run_mode.triggering_label,
        triggering_value = run_mode.triggering_value,
    )

def _cq_tryjob_builder(verifier, cq_group, project, seen):
    """cq_tryjob_verifier(...) => cq_pb.Verifiers.Tryjob.Builder.

    Args:
      verifier: luci.cq_tryjob_verifier node.
      cq_group: luci.cq_group node (for error messages).
      project: luci.project node (for project name).
      seen: map[builder name as in *.cfg => cq.cq_tryjob_verifier that added it].
    """
    builder = _cq_builder_from_node(verifier)
    name = _cq_builder_name(builder, project)

    # Make sure there are no dups.
    if name in seen:
        error(
            "verifier that references %s was already added to %s, previous declaration:\n%s" %
            (builder, cq_group, seen[name].trace),
            trace = verifier.trace,
        )
        return None
    seen[name] = verifier

    return cq_pb.Verifiers.Tryjob.Builder(
        name = name,
        result_visibility = _cq_visibility(verifier.props.result_visibility),
        includable_only = verifier.props.includable_only,
        cancel_stale = _cq_toggle(verifier.props.cancel_stale),
        disable_reuse = verifier.props.disable_reuse,
        experiment_percentage = verifier.props.experiment_percentage,
        owner_whitelist_group = verifier.props.owner_whitelist,
        location_filters = [_cq_location_filter(n) for n in verifier.props.location_filters],
        equivalent_to = _cq_equivalent_to(verifier, project),
        mode_allowlist = verifier.props.mode_allowlist,
    )

def _cq_builder_from_node(node):
    """Given a CQ node returns corresponding 'builder' node by following refs.

    Args:
      node: either 'cq_tryjob_verifier' or 'cq_equivalent_builder' node.

    Returns:
      Corresponding 'builder' node.
    """

    # Per cq_tryjob_verifier implementation, the node MUST have single
    # builder_ref child, which we resolve to a concrete luci.builder(...) node.
    refs = graph.children(node.key, kinds.BUILDER_REF)
    if len(refs) != 1:
        fail("impossible result %s" % (refs,))
    return builder_ref.follow(refs[0], context_node = node)

def _cq_equivalent_to(verifier, project):
    """cq_tryjob_verifier(...) => cq_pb.Verifiers.Tryjob.EquivalentBuilder | None.

    Args:
      verifier: 'cq_tryjob_verifier' node.
      project: 'project' node.
    """
    nodes = graph.children(verifier.key, kind = kinds.CQ_EQUIVALENT_BUILDER)
    if len(nodes) == 0:
        return None
    if len(nodes) > 1:
        fail("impossible result %s" % (nodes,))
    equiv_builder = nodes[0]
    return cq_pb.Verifiers.Tryjob.EquivalentBuilder(
        name = _cq_builder_name(_cq_builder_from_node(equiv_builder), project),
        percentage = equiv_builder.props.percentage,
        owner_whitelist_group = equiv_builder.props.whitelist,
    )

def _cq_location_filter(node):
    """cq.location_filter(...) => cq_pb.Verifiers.Tryjob.Builder.LocationFilter"""
    return cq_pb.Verifiers.Tryjob.Builder.LocationFilter(
        gerrit_host_regexp = node.gerrit_host_regexp or ".*",
        gerrit_project_regexp = node.gerrit_project_regexp or ".*",
        gerrit_ref_regexp = node.gerrit_ref_regexp or ".*",
        path_regexp = node.path_regexp or ".*",
        exclude = node.exclude,
    )

def _cq_builder_name(builder, project):
    """Given Builder node returns a string reference to it for CQ config."""
    return "%s/%s/%s" % (
        builder.props.project or project.props.name,
        builder.props.bucket,
        builder.props.name,
    )

def _cq_toggle(val):
    """Bool|None => cq_pb.Toggle."""
    if val == None:
        return cq_pb.UNSET
    return cq_pb.YES if val else cq_pb.NO

def _cq_visibility(val):
    """Visibility|None => cq_pb.Visibility."""
    if val == None:
        return cq_pb.COMMENT_LEVEL_UNSET
    return val

def _cq_user_limit(limit):
    """cq.user_limit(...) => cq_pb.UserLimit."""
    return cq_pb.UserLimit(
        name = limit.name,
        principals = limit.principals,
        run = _cq_user_limit_run(limit.run),
    )

def _cq_user_limit_run(limits):
    """cq.run_limits(...) => cq_pb.UserLimit.Run."""
    return cq_pb.UserLimit.Run(
        max_active = _cq_user_limit_limit(
            limits.max_active if limits != None else None,
        ),
        reach_limit_msg = limits.reach_limit_msg if limits != None else None,
    )

def _cq_user_limit_limit(limit):
    """Int|None => cq_pb.UserLimit.Limit."""

    # if the limit is None, return with unlimited = True, so that
    # so that the config output clarifies what limits were set as unlimited.
    if limit == None:
        return cq_pb.UserLimit.Limit(unlimited = True)
    return cq_pb.UserLimit.Limit(value = limit)

################################################################################
## notify.cfg.

def gen_notify_cfg(ctx):
    """Generates notify.cfg.

    Args:
      ctx: the generator context.
    """
    opts = graph.node(keys.notify())
    tree_closing_enabled = opts.props.tree_closing_enabled if opts else False

    notifiables = graph.children(keys.project(), kinds.NOTIFIABLE)
    templates = graph.children(keys.project(), kinds.NOTIFIER_TEMPLATE)
    if not notifiables and not templates:
        return

    service = get_service("notify", "using notifiers or tree closers")

    # Write all defined templates.
    for t in templates:
        path = "%s/email-templates/%s.template" % (service.app_id, t.props.name)
        set_config(ctx, path, t.props.body)

    # Build the map 'builder node => [notifiable node] watching it'.
    per_builder = {}
    for n in notifiables:
        for ref in graph.children(n.key, kinds.BUILDER_REF):
            builder = builder_ref.follow(ref, context_node = n)
            per_builder.setdefault(builder, []).append(n)

    # Calculate the map {builder key => [gitiles_poller that triggers it]}
    # needed for deriving repo URLs associated with builders.
    #
    # TODO(vadimsh): Cache this somewhere. A very similar calculation is done by
    # scheduler.cfg generator. There's currently no reliable mechanism to carry
    # precalculated state between different luci.generator(...) implementations.
    pollers = _notify_pollers_map()

    # Emit a single notify_pb.Notifier per builder with all notifications for
    # that particular builder.
    notifiers_pb = []
    for builder, nodes in per_builder.items():
        # 'nodes' here is a list of luci.notifiable nodes. Categorize them
        # either into luci.notifier or luci.tree_closer based on their 'kind'
        # property.
        notifications = [n for n in nodes if n.props.kind == "luci.notifier"]
        tree_closers = [n for n in nodes if n.props.kind == "luci.tree_closer"]
        if len(notifications) + len(tree_closers) != len(nodes):
            fail("impossible")

        # Validate luci.notifier(...) has enough information about builders.
        repo = _notify_builder_repo(builder, pollers)
        if any([n.props.notify_blamelist for n in notifications]) and not repo:
            error(
                ("cannot deduce a primary repo for %s, which is observed by a " +
                 "luci.notifier with notify_blamelist=True; add repo=... field") % builder,
                trace = builder.trace,
            )

        notifiers_pb.append(notify_pb.Notifier(
            notifications = [_notify_notification_pb(n) for n in notifications],
            tree_closers = [_notify_tree_closer_pb(n) for n in tree_closers],
            builders = [notify_pb.Builder(
                bucket = builder.props.bucket,
                name = builder.props.name,
                repository = repo,
            )],
        ))

    # Done!
    set_config(ctx, service.cfg_file, notify_pb.ProjectConfig(
        notifiers = sorted(
            notifiers_pb,
            key = lambda n: (n.builders[0].bucket, n.builders[0].name),
        ),
        tree_closing_enabled = tree_closing_enabled,
    ))

def _notify_used_template_name(node):
    """Given a luci.notifiable node returns a name of a template it references."""
    templs = graph.children(node.key, kind = kinds.NOTIFIER_TEMPLATE)
    if len(templs) == 0:
        return None
    if len(templs) == 1:
        return templs[0].props.name
    fail("impossible")

def _notify_notification_pb(node):
    """Given a luci.notifiable node returns notify_pb.Notification."""
    pb = notify_pb.Notification(
        on_occurrence = node.props.on_occurrence,
        on_new_status = node.props.on_new_status,
        failed_step_regexp = node.props.failed_step_regexp,
        failed_step_regexp_exclude = node.props.failed_step_regexp_exclude,
        template = _notify_used_template_name(node),

        # deprecated
        on_change = node.props.on_status_change,
        on_failure = node.props.on_failure,
        on_new_failure = node.props.on_new_failure,
        on_success = node.props.on_success,
    )
    if node.props.notify_emails or node.props.notify_rotation_urls:
        pb.email = notify_pb.Notification.Email(
            recipients = node.props.notify_emails,
            rotation_urls = node.props.notify_rotation_urls,
        )
    if node.props.notify_blamelist:
        pb.notify_blamelist = notify_pb.Notification.Blamelist(
            repository_allowlist = node.props.blamelist_repos_whitelist,
        )
    return pb

def _notify_tree_closer_pb(node):
    """Given a luci.notifiable node returns notify_pb.TreeCloser."""
    return notify_pb.TreeCloser(
        tree_name = node.props.tree_name,
        tree_status_host = node.props.tree_status_host,
        failed_step_regexp = node.props.failed_step_regexp,
        failed_step_regexp_exclude = node.props.failed_step_regexp_exclude,
        template = _notify_used_template_name(node),
    )

def _notify_pollers_map():
    """Returns a map {builder key => [gitiles poller that triggers it]}."""
    out = {}
    for bucket in get_buckets():
        for poller in graph.children(bucket.key, kinds.GITILES_POLLER):
            for builder in triggerer.targets(poller):
                out.setdefault(builder.key, []).append(poller)
    return out

def _notify_builder_repo(builder, pollers_map):
    """Given a builder node returns its primary repo URL or None.

    Either uses a repo URL explicitly passed via `repo` field in the builder
    definition, or grabs it from a poller that triggers this builder, if there's
    only one such poller (so there's no ambiguity).
    """
    if builder.props.repo:
        return builder.props.repo
    repos = set([t.props.repo for t in pollers_map.get(builder.key, [])])
    if len(repos) == 1:
        return list(repos)[0]
    return None

################################################################################
## tricium.cfg.

def gen_tricium_cfg(ctx):
    """Generates tricium.cfg.

    Args:
      ctx: the generator context.
    """
    cq_groups = graph.children(keys.project(), kind = kinds.CQ_GROUP)
    if not cq_groups:
        return

    project = get_project()
    result = None
    for cq_group in cq_groups:
        tricium_config = _tricium_config(
            graph.children(cq_group.key, kind = kinds.CQ_TRYJOB_VERIFIER),
            cq_group,
            project,
        )
        if tricium_config == None:
            continue
        elif result == None:
            result = struct(cfg = tricium_config, mapping_cq_group = cq_group)
        elif result.cfg != tricium_config:
            # if multiple config groups have defined Tricium analyzers, they
            # MUST generate the same Tricium project config.
            error(
                "%s is watching different set of Gerrit repos or defining different analyzers from %s" % (cq_group, result.mapping_cq_group),
                trace = cq_group.trace,
            )

    if result:
        service = get_service("tricium", "defining Tricium project config")
        result.cfg.service_account = "%s@appspot.gserviceaccount.com" % service.app_id
        set_config(ctx, service.cfg_file, result.cfg)

def _tricium_config(verifiers, cq_group, project):
    """Returns tricium_pb.ProjectConfig.

    Returns None if none of the provided verifiers is a Tricium analyzer.

    Args:
      verifiers: a list of luci.cq_tryjob_verifier nodes.
      cq_group: a luci.cq_group node.
      project: a luci.project node.
    """
    ret = tricium_pb.ProjectConfig()
    whitelisted_group = None
    watching_gerrit_projects = None
    disable_reuse = None
    for verifier in verifiers:
        if cq.MODE_ANALYZER_RUN not in verifier.props.mode_allowlist:
            continue
        recipe = _tricium_recipe(verifier, project)
        func_name = _compute_func_name(recipe)
        gerrit_projs, exts = _parse_location_filters_for_tricium(verifier.props.location_filters)
        if watching_gerrit_projects == None:
            watching_gerrit_projects = gerrit_projs
        elif watching_gerrit_projects != gerrit_projs:
            error(
                "The location_filters of analyzer %s specifies a different set of Gerrit repos from the other analyzer; got: %s other: %s" % (
                    verifier,
                    ["%s-review.googlesource.com/%s" % (host, proj) for host, proj in gerrit_projs],
                    ["%s-review.googlesource.com/%s" % (host, proj) for host, proj in watching_gerrit_projects],
                ),
                trace = verifier.trace,
            )
        verifier_disable_reuse = verifier.props.disable_reuse
        if disable_reuse == None:
            disable_reuse = verifier_disable_reuse
        elif disable_reuse != verifier_disable_reuse:
            error(
                "The disable_reuse field of analyzer %s does not match the others, got: %s, others: %s" % (
                    verifier,
                    verifier_disable_reuse,
                    disable_reuse,
                ),
                trace = verifier.trace,
            )
        ret.functions.append(tricium_pb.Function(
            type = tricium_pb.Function.ANALYZER,
            name = func_name,
            needs = tricium_pb.GIT_FILE_DETAILS,
            provides = tricium_pb.RESULTS,
            path_filters = ["*.%s" % ext for ext in exts],
            impls = [
                tricium_pb.Impl(
                    provides_for_platform = tricium_pb.LINUX,
                    runtime_platform = tricium_pb.LINUX,
                    recipe = recipe,
                ),
            ],
        ))
        ret.selections.append(tricium_pb.Selection(
            function = func_name,
            platform = tricium_pb.LINUX,
        ))
        owner_whitelist = sorted(verifier.props.owner_whitelist) if verifier.props.owner_whitelist else []
        if whitelisted_group == None:
            whitelisted_group = owner_whitelist
        elif whitelisted_group != owner_whitelist:
            error(
                "analyzer %s has different owner_whitelist from other analyzers in the config group %s" % (verifier, cq_group),
                trace = verifier.trace,
            )

    if not ret.functions:
        return None

    ret.functions = sorted(ret.functions, key = lambda f: f.name)
    ret.selections = sorted(ret.selections, key = lambda f: f.function)
    watching_gerrit_projects = watching_gerrit_projects or sorted(set([
        (w.__gob_host, w.__gob_proj)
        for w in cq_group.props.watch
    ]))
    ret.repos = [
        tricium_pb.RepoDetails(
            gerrit_project = tricium_pb.RepoDetails.GerritProject(
                host = "%s-review.googlesource.com" % host,
                project = proj,
                git_url = "https://%s.googlesource.com/%s" % (host, proj),
            ),
            whitelisted_group = whitelisted_group,
            check_all_revision_kinds = disable_reuse,
        )
        for host, proj in watching_gerrit_projects
    ]
    return ret

def _tricium_recipe(verifier, project):
    """(luci.cq_tryjob_verifier, luci.project) => tricium_pb.Recipe."""
    builder = _cq_builder_from_node(verifier)
    return tricium_pb.Recipe(
        project = builder.props.project or project.props.name,
        bucket = builder.props.bucket,
        builder = builder.props.name,
    )

def _compute_func_name(recipe):
    """Returns an alphanumeric function name."""

    def normalize(s):
        return "".join([ch for ch in s.title().elems() if ch.isalnum()])

    return "".join([
        normalize(recipe.project),
        normalize(recipe.bucket),
        normalize(recipe.builder),
    ])

def _parse_location_filters_for_tricium(location_filters):
    """Returns Gerrit projects and path filters based on location_filters.

    The parsed host, project, and extension patterns are used only for
    generating watched repos and path filters in the Tricium config. Hosts,
    projects or path filters that aren't in the expected format will be
    skipped.

    Returns:
        A pair: (list of (host, proj) pairs, list of extension patterns).
        Either list may be empty.
    """
    host_and_projs = []
    exts = []
    for f in location_filters:
        if f.gerrit_host_regexp.endswith("-review.googlesource.com") and f.gerrit_project_regexp:
            host = f.gerrit_host_regexp[:-len("-review.googlesource.com")]
            proj = f.gerrit_project_regexp
            host_and_projs.append((host, proj))
        if f.path_regexp and f.path_regexp.startswith(r".+\."):
            exts.append(f.path_regexp[len(r".+\."):])
    return sorted(set(host_and_projs)), sorted(set(exts))

def _buildbucket_dynamic_builder_template(bucket):
    """luci.bucket(...) node => buildbucket_pb.Bucket.DynamicBuilderTemplate or None."""
    if not bucket.props.dynamic and len(graph.children(bucket.key, kinds.DYNAMIC_BUILDER_TEMPLATE)) == 0:
        return None

    if not bucket.props.dynamic and len(graph.children(bucket.key, kinds.DYNAMIC_BUILDER_TEMPLATE)) > 0:
        error("bucket \"%s\" must not have dynamic_builder_template" % bucket.props.name, trace = bucket.trace)

    if len(graph.children(bucket.key, kinds.DYNAMIC_BUILDER_TEMPLATE)) > 1:
        error("dynamic bucket \"%s\" can have at most one dynamic_builder_template" % bucket.props.name, trace = bucket.trace)

    bldr_template = None
    for node in graph.children(bucket.key, kinds.DYNAMIC_BUILDER_TEMPLATE):
        bldr_template, _ = _buildbucket_builder(node, None)

    return buildbucket_pb.Bucket.DynamicBuilderTemplate(template = bldr_template)
