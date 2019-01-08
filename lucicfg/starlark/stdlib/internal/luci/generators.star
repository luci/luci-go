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

load('@stdlib//internal/generator.star', 'generator')
load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/luci/common.star', 'keys', 'kinds', 'triggerer')
load('@stdlib//internal/luci/lib/acl.star', 'acl', 'aclimpl')

load('@proto//luci/logdog/project_config.proto', logdog_pb='svcconfig')
load('@proto//luci/buildbucket/project_config.proto', buildbucket_pb='buildbucket')
load('@proto//luci/config/project_config.proto', config_pb='config')


def register():
  """Registers all LUCI config generator callbacks."""
  generator(impl = gen_project_cfg)
  generator(impl = gen_logdog_cfg)
  generator(impl = gen_buildbucket_cfg)
  generator(impl = gen_scheduler_cfg)


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
        'missing %r in core.project(...), it is required for %s' % (kind, why),
        trace=proj.trace,
    )
  return svc


def get_buckets():
  """Returns all defined bucket() nodes, if any."""
  return graph.children(keys.project(), kinds.BUCKET)


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
_bb_roles = {
    acl.BUILDBUCKET_READER: buildbucket_pb.Acl.READER,
    acl.BUILDBUCKET_SCHEDULER: buildbucket_pb.Acl.SCHEDULER,
    acl.BUILDBUCKET_WRITER: buildbucket_pb.Acl.WRITER,
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
    cfg.acl_sets.append(buildbucket_pb.AclSet(
        name = bucket.props.name,
        acls = gen_buildbucket_acls(bucket),
    ))
    cfg.buckets.append(buildbucket_pb.Bucket(
        name = bucket.props.name,
        acl_sets = [bucket.props.name],
        swarming = buildbucket_pb.Swarming(
            builders = gen_buildbucket_builders(bucket, swarming.host),
        ),
    ))


def gen_buildbucket_acls(bucket):
  """core.bucket(...) node => [buildbucket_pb.Acl]."""
  return [
      buildbucket_pb.Acl(
          role = _bb_roles[a.role],
          group = a.group,
          identity = 'user:' + a.user if a.user else None,
      )
      for a in filter_acls(get_bucket_acls(bucket), _bb_roles.keys())
  ]


def gen_buildbucket_builders(bucket, swarming_host):
  """core.bucket(...) node => [buildbucket_pb.Builder]."""
  out = []
  for node in graph.children(bucket.key, kinds.BUILDER):
    # TODO(vadimsh): Fill in all properties.
    b = buildbucket_pb.Builder(
        name = node.props.name,
        swarming_host = swarming_host,
        swarming_tags = [],
        dimensions = [],
        recipe = None,
        priority = 0,
        execution_timeout_secs = 0,
        expiration_secs = 0,
        caches = [],
        build_numbers = 0,
        service_account = '',
        auto_builder_dimension = 0,
        experimental = 0,
        luci_migration_host = "",
        task_template_canary_percentage = None,
    )
    # TODO(vadimsh): Apply recipe-based mutators to e.g. add named caches the
    # recipe depends on.
    out.append(b)
  return out


################################################################################
## scheduler.cfg.


def gen_scheduler_cfg(ctx):
  """Generates scheduler.cfg."""
  buckets = get_buckets()
  if not buckets:
    return

  # Discover who triggers who, validate there are no ambiguities in 'triggers'
  # and 'triggered_by' relations (triggerer.targets reports them as errors).

  pollers = {}   # GITILES_POLLER node -> list of BUILDER nodes it triggers.
  builders = {}  # BUILDER node -> list of BUILDER nodes it triggers.

  for bucket in buckets:
    for poller in graph.children(bucket.key, kinds.GITILES_POLLER):
      # Note: the list of targets may be empty. We still want to define a
      # poller, so add the entry to the dict regardless. This may be useful to
      # confirm a poller works before making it trigger anything.
      pollers[poller] = triggerer.targets(poller)

    for builder in graph.children(bucket.key, kinds.BUILDER):
      targets = triggerer.targets(builder)
      if targets:
        builders[builder] = targets

  # The scheduler service is not used at all? Don't require its hostname then.
  if not pollers and not builders:
    return

  # TODO(vadimsh): Actually generate configs:
  #   1. Add per-bucket ACL sets to use as base ACLs.
  #   2. Add gitiles Trigger entry for each key in 'pollers' dict.
  #   3. Add Job entry for each builder appearing in 'pollers' and 'builders'.
  #   4. Add Job-scoped TRIGGERER ACL per 'builders' map.
