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

load('@stdlib//internal/error.star', 'error')
load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/lucicfg.star', 'lucicfg')
load('@stdlib//internal/time.star', 'time')

load('@stdlib//internal/luci/common.star', 'builder_ref', 'keys', 'kinds', 'triggerer')
load('@stdlib//internal/luci/lib/acl.star', 'acl', 'aclimpl')

load('@proto//google/protobuf/wrappers.proto', wrappers_pb='google.protobuf')

load('@proto//luci/buildbucket/project_config.proto', buildbucket_pb='buildbucket')
load('@proto//luci/config/project_config.proto', config_pb='config')
load('@proto//luci/logdog/project_config.proto', logdog_pb='svcconfig')
load('@proto//luci/milo/project_config.proto', milo_pb='milo')
load('@proto//luci/scheduler/project_config.proto', scheduler_pb='scheduler.config')


def register():
  """Registers all LUCI config generator callbacks."""
  lucicfg.generator(impl = gen_project_cfg)
  lucicfg.generator(impl = gen_logdog_cfg)
  lucicfg.generator(impl = gen_buildbucket_cfg)
  lucicfg.generator(impl = gen_scheduler_cfg)
  lucicfg.generator(impl = gen_milo_cfg)


################################################################################
## Utilities to be used from generators.


def get_project(required=True):
  """Returns project() node or fails if it wasn't declared."""
  n = graph.node(keys.project())
  if not n and required:
    fail('project(...) definition is missing, it is required')
  return n


def get_service(kind, why):
  """Returns service struct (see service.star), reading it from project node."""
  proj = get_project()
  svc = getattr(proj.props, kind)
  if not svc:
    fail(
        'missing %r in luci.project(...), it is required for %s' % (kind, why),
        trace=proj.trace,
    )
  return svc


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


def optional_sec(duration):
  """duration|None => number of seconds | None."""
  return None if duration == None else duration/time.second


def optional_UInt32Value(val):
  """int|None => google.protobuf.UInt32Value."""
  return None if val == None else wrappers_pb.UInt32Value(value=val)


################################################################################
## project.cfg.


def gen_project_cfg(ctx):
  """Generates project.cfg."""
  # lucicfg is allowed to interpret *.star files without any actual definitions.
  # This is used in tests, for example. If there's no project(...) rule, but
  # there are some other LUCI definitions, the corresponding generators will
  # fail on their own in get_project() calls.
  proj = get_project(required=False)
  if not proj:
    return

  # Use "projects/<name>" as default value for lucicfg.config(config_set=...).
  # This is noop if config_set was already explicitly provided.
  __native__.set_meta_default('config_set', 'projects/%s' % proj.props.name)

  # Find all PROJECT_CONFIGS_READER role entries.
  access = []
  for a in filter_acls(get_project_acls(), [acl.PROJECT_CONFIGS_READER]):
    if a.user:
      access.append('user:' + a.user)
    elif a.group:
      access.append('group:' + a.group)

  ctx.config_set['project.cfg'] = config_pb.ProjectCfg(
      name = proj.props.name,
      access = access,
  )


################################################################################
## logdog.cfg.


def gen_logdog_cfg(ctx):
  """Generates logdog.cfg."""
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

  logdog = get_service('logdog', 'defining LogDog options')
  ctx.config_set[logdog.cfg_file] = logdog_pb.ProjectConfig(
      reader_auth_groups = readers,
      writer_auth_groups = writers,
      archive_gs_bucket = opts.props.gs_bucket,
  )


################################################################################
## buildbucket.cfg.


# acl.role => buildbucket_pb.Acl.Role.
_buildbucket_roles = {
    acl.BUILDBUCKET_READER: buildbucket_pb.Acl.READER,
    acl.BUILDBUCKET_TRIGGERER: buildbucket_pb.Acl.SCHEDULER,
    acl.BUILDBUCKET_OWNER: buildbucket_pb.Acl.WRITER,
}


def gen_buildbucket_cfg(ctx):
  """Generates buildbucket.cfg."""
  buckets = get_buckets()
  if not buckets:
    return

  buildbucket = get_service('buildbucket', 'defining buckets')
  swarming = get_service('swarming', 'defining builders')

  cfg = buildbucket_pb.BuildbucketCfg()
  ctx.config_set[buildbucket.cfg_file] = cfg

  for bucket in buckets:
    cfg.buckets.append(buildbucket_pb.Bucket(
        name = bucket.props.name,
        acls = _buildbucket_acls(get_bucket_acls(bucket)),
        swarming = buildbucket_pb.Swarming(
            builders = _buildbucket_builders(bucket, swarming.host),
        ),
    ))


def _buildbucket_acls(elementary):
  """[acl.elementary] => filtered [buildbucket_pb.Acl]."""
  return [
      buildbucket_pb.Acl(
          role = _buildbucket_roles[a.role],
          group = a.group,
          identity = 'user:' + a.user if a.user else None,
      )
      for a in filter_acls(elementary, _buildbucket_roles.keys())
  ]


def _buildbucket_builders(bucket, swarming_host):
  """luci.bucket(...) node => [buildbucket_pb.Builder]."""
  out = []
  for node in graph.children(bucket.key, kinds.BUILDER):
    out.append(buildbucket_pb.Builder(
        name = node.props.name,
        swarming_host = swarming_host,
        recipe = _buildbucket_recipe(node),
        service_account = node.props.service_account,
        caches = _buildbucket_caches(node.props.caches),
        execution_timeout_secs = optional_sec(node.props.execution_timeout),
        dimensions = _buildbucket_dimensions(node.props.dimensions),
        priority = node.props.priority,
        swarming_tags = node.props.swarming_tags,
        expiration_secs = optional_sec(node.props.expiration_timeout),
        build_numbers = _buildbucket_toggle(node.props.build_numbers),
        experimental = _buildbucket_toggle(node.props.experimental),
        luci_migration_host = node.props.luci_migration_host,
        task_template_canary_percentage = optional_UInt32Value(
            node.props.task_template_canary_percentage),
    ))
  return out


def _buildbucket_recipe(node):
  """Builder node => buildbucket_pb.Builder.Recipe."""
  recipes = graph.children(node.key, kinds.RECIPE)
  if len(recipes) != 1:
    fail('impossible: the builder should have a reference to a recipe')
  recipe = recipes[0]
  return buildbucket_pb.Builder.Recipe(
      name = recipe.props.recipe,
      cipd_package = recipe.props.cipd_package,
      cipd_version = recipe.props.cipd_version,
      properties_j = sorted([
          '%s:%s' % (k, to_json(v)) for k, v in node.props.properties.items()
      ]),
  )


def _buildbucket_caches(caches):
  """[swarming.cache] => [buildbucket_pb.Builder.CacheEntry]."""
  out = []
  for c in caches:
    out.append(buildbucket_pb.Builder.CacheEntry(
        name = c.name,
        path = c.path,
        wait_for_warm_cache_secs = optional_sec(c.wait_for_warm_cache),
    ))
  return sorted(out, key=lambda x: x.name)


def _buildbucket_dimensions(dims):
  """{str: [swarming.dimension]} => [str] for 'dimensions' field."""
  out = []
  for key in sorted(dims):
    for d in dims[key]:
      if d.expiration == None:
        out.append('%s:%s' % (key, d.value))
      else:
        out.append('%d:%s:%s' % (d.expiration/time.second, key, d.value))
  return out


def _buildbucket_toggle(val):
  """Bool|None => buildbucket_pb.Toggle."""
  if val == None:
    return buildbucket_pb.UNSET
  return buildbucket_pb.YES if val else buildbucket_pb.NO


################################################################################
## scheduler.cfg.


# acl.role => scheduler_pb.Acl.Role.
_scheduler_roles = {
    acl.SCHEDULER_READER: scheduler_pb.Acl.READER,
    acl.SCHEDULER_TRIGGERER: scheduler_pb.Acl.TRIGGERER,
    acl.SCHEDULER_OWNER: scheduler_pb.Acl.OWNER,
}


def gen_scheduler_cfg(ctx):
  """Generates scheduler.cfg."""
  buckets = get_buckets()
  if not buckets:
    return

  # Discover who triggers who, validate there are no ambiguities in 'triggers'
  # and 'triggered_by' relations (triggerer.targets reports them as errors).

  pollers = {}   # GITILES_POLLER node -> list of BUILDER nodes it triggers.
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
      # poller, so add the entry to the dict regardless. This may be useful to
      # confirm a poller works before making it trigger anything.
      pollers[poller] = triggerer.targets(poller)
      for target in pollers[poller]:
        add_triggered_by(target, triggered_by=poller)

    for builder in graph.children(bucket.key, kinds.BUILDER):
      targets = triggerer.targets(builder)
      if targets and not builder.props.service_account:
        error(
            '%s needs service_account set, it triggers other builders: %s' %
                (builder, ', '.join([str(t) for t in targets])),
            trace=builder.trace,
        )
      else:
        for target in targets:
          add_triggered_by(target, triggered_by=builder)
      # If using a cron schedule or a custom triggering policy, need to setup a
      # Job entity for this builder.
      if builder.props.schedule or builder.props.triggering_policy:
        want_scheduler_job_for(builder)

  # List of BUILDER and GITILES_POLLER nodes we need an entity in the scheduler
  # config for.
  entities = pollers.keys() + builders.keys()

  # The scheduler service is not used at all? Don't require its hostname then.
  if not entities:
    return

  scheduler = get_service('scheduler', 'using triggering or pollers')
  buildbucket = get_service('buildbucket', 'using triggering')

  cfg = scheduler_pb.ProjectConfig()
  ctx.config_set[scheduler.cfg_file] = cfg

  # Generate per-bucket ACL sets based on bucket-level permissions. Skip buckets
  # that aren't used to avoid polluting configs with unnecessary entries.
  for bucket in get_buckets_of(entities):
    cfg.acl_sets.append(scheduler_pb.AclSet(
        name = bucket.props.name,
        acls = _scheduler_acls(get_bucket_acls(bucket)),
    ))

  # We prefer to use bucket-less names in the scheduler configs, so that IDs
  # that show up in logs and in the debug UI match what is used in the starlark
  # config. On conflicts, append the bucket name as suffix to disambiguate.
  #
  # TODO(vadimsh): Revisit this if LUCI Scheduler starts supporting buckets
  # directly. Right now each project has a single flat namespace of job IDs and
  # all existing configs (known to me) use 'scheduler job name == builder name'.
  # Artificially injecting bucket names into all job IDs will complicate the
  # migration to lucicfg by obscuring diffs between existing configs and new
  # generated configs.
  node_to_id = _scheduler_disambiguate_ids(entities)

  # Add Trigger entry for each gitiles poller. Sort according to final IDs.
  for poller, targets in pollers.items():
    cfg.trigger.append(scheduler_pb.Trigger(
        id = node_to_id[poller],
        acl_sets = [poller.props.bucket],
        triggers = [node_to_id[b] for b in targets],
        schedule = poller.props.schedule,
        gitiles = scheduler_pb.GitilesTask(
            repo = poller.props.repo,
            refs = ['regexp:' + r for r in poller.props.refs],
        ),
    ))
  cfg.trigger = sorted(cfg.trigger, key=lambda x: x.id)

  # Add Job entry for each triggered or cron builder. Grant all triggering
  # builders (if any) TRIGGERER role. Sort according to final IDs.
  for builder, triggered_by in builders.items():
    cfg.job.append(scheduler_pb.Job(
        id = node_to_id[builder],
        acl_sets = [builder.props.bucket],
        acls = _scheduler_acls(aclimpl.normalize_acls([
            acl.entry(acl.SCHEDULER_TRIGGERER, users=t.props.service_account)
            for t in triggered_by if t.key.kind == kinds.BUILDER
        ])),
        schedule = builder.props.schedule,
        triggering_policy = builder.props.triggering_policy,
        buildbucket = scheduler_pb.BuildbucketTask(
            server = buildbucket.host,
            bucket = builder.props.bucket,
            builder = builder.props.name,
        ),
    ))
  cfg.job = sorted(cfg.job, key=lambda x: x.id)


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
  used =  {}  # name -> node, reverse of 'names'

  # use_name deals with the second layer of ambiguities: when our generated
  # '<name>-<bucket>' name happened to clash with some other '<name>'. This
  # should be super rare. Lack of 'while' in starlark makes it difficult to
  # handle such collisions automatically, so we just give up and ask the user
  # to pick some other names.
  def use_name(name, node):
    if name in used:
      fail((
          '%s and %s cause ambiguities in the scheduler config file, ' +
          'pick names that don\'t start with a bucket name') % (node, used[name]),
          trace=node.trace,
      )
    names[node] = name
    used[name] = node

  for nm, candidates in claims.items():
    if len(candidates) == 1:
      use_name(nm, candidates[0])
    else:
      for node in candidates:
        use_name('%s-%s' % (node.props.bucket, nm), node)

  return names


def _scheduler_acls(elementary):
  """[acl.elementary] => filtered [scheduler_pb.Acl]."""
  return [
      scheduler_pb.Acl(
          role = _scheduler_roles[a.role],
          granted_to = a.user if a.user else 'group:' + a.group,
      )
      for a in filter_acls(elementary, _scheduler_roles.keys())
  ]


################################################################################
## milo.cfg.


def gen_milo_cfg(ctx):
  """Generates milo.cfg."""
  _milo_check_connections()

  # Note: luci.milo(...) node is optional.
  milo_node = graph.node(keys.milo())
  opts = struct(
      logo = milo_node.props.logo if milo_node else None,
      favicon = milo_node.props.favicon if milo_node else None,
  )

  # Keep the order of views as they were defined, for Milo's list of consoles.
  views = graph.children(keys.project(), kinds.MILO_VIEW, order_by=graph.DEFINITION_ORDER)
  if not views and not milo_node:
    return

  build_bug_template = None
  if milo_node and milo_node.props.monorail_project:
    build_bug_template = milo_pb.BugTemplate(
        summary = milo_node.props.bug_summary,
        description = milo_node.props.bug_description,
        monorail_project = milo_node.props.monorail_project,
        components = milo_node.props.monorail_components,
    )

  milo = get_service('milo', 'using views or setting Milo config')
  project_name = get_project().props.name

  ctx.config_set[milo.cfg_file] = milo_pb.Project(
      build_bug_template = build_bug_template,
      logo_url = opts.logo,
      consoles = [
          _milo_console_pb(view, opts, project_name)
          for view in views
      ],
  )


def _milo_check_connections():
  """Ensures all *_view_entry are connected to one and only one *_view."""
  root = keys.milo_entries_root()
  for e in graph.children(root):
    views = [p for p in graph.parents(e.key) if p.key != root]
    if len(views) == 0:
      error('%s is not added to any view, either remove or comment it out' % e, trace=e.trace)
    elif len(views) > 1:
      error(
          '%s is added to multiple views: %s' %
          (e, ', '.join([str(v) for v in views])),
          trace=e.trace)


def _milo_console_pb(view, opts, project_name):
  """Given MILO_VIEW node returns milo_pb.Console."""
  ch = graph.children(view.key)
  if len(ch) != 1:
    fail('impossible: %s' % (ch,))
  view = ch[0]
  if view.key.kind == kinds.LIST_VIEW:
    return _milo_list_view(view, opts, project_name)
  if view.key.kind == kinds.CONSOLE_VIEW:
    return _milo_console_view(view, opts, project_name)
  fail('impossible: %s' % (view,))


def _milo_list_view(view, opts, project_name):
  """Given a LIST_VIEW node produces milo_pb.Console."""
  builders = []
  seen = {}
  for e in graph.children(view.key, order_by=graph.DEFINITION_ORDER):
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
  for e in graph.children(view.key, order_by=graph.DEFINITION_ORDER):
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
      refs = ['regexp:' + r for r in view.props.refs],
      exclude_ref = view.props.exclude_ref,

      # TODO(hinoka,iannucci): crbug/832893 - Support custom manifest names,
      # such as 'UNPATCHED' / 'PATCHED'.
      manifest_name = 'REVISION',

      include_experimental_builds = view.props.include_experimental_builds,
      favicon_url = view.props.favicon or opts.favicon,
      builders = builders,
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
  if entry.props.buildbot:
    builder_pb.name.append('buildbot/%s' % entry.props.buildbot)

  # Note: this is a one-item list for regular entries and an empty list for
  # entries that have only deprecated Buildbot references. Other cases are
  # impossible per *_view_entry implementation.
  refs = graph.children(entry.key, kinds.BUILDER_REF)
  if len(refs) > 1:
    fail('impossible result %s' % (refs,))
  if len(refs) == 0:
    return builder_pb  # pure Buildbot entry, we don't care much for them

  # Grab BUILDER node and ensure it hasn't be added to this view yet.
  builder = builder_ref.follow(refs[0], context_node=entry)
  if builder.key in seen:
    error(
        'builder %s was already added to %s, previous declaration:\n%s' %
        (builder, view, seen[builder.key].trace),
        trace=entry.trace)
    return None
  seen[builder.key] = entry

  builder_pb.name.append('buildbucket/%s/%s' % (
      _milo_bucket_name(builder.props.bucket, project_name),
      builder.props.name,
  ))
  return builder_pb


def _milo_bucket_name(bucket_name, project_name):
  """Prefixes the bucket name with `luci.<project>.` if necessary."""
  pfx = 'luci.%s.' % project_name
  return bucket_name if bucket_name.startswith(pfx) else pfx + bucket_name
