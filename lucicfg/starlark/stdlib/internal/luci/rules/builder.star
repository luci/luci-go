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

load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/luci/common.star', 'builder_ref', 'keys', 'triggerer')
load('@stdlib//internal/luci/lib/validate.star', 'validate')
load('@stdlib//internal/luci/lib/swarming.star', 'swarming')


def builder(
      name,
      bucket,
      recipe,

      # Run what and how.
      properties=None,
      service_account=None,
      caches=None,
      execution_timeout=None,

      # Scheduling parameters.
      dimensions=None,
      priority=None,
      tags=None,
      expiration=None,

      # Tweaks.
      build_numbers=None,
      experimental=None,
      task_template_canary_percentage=None,

      # Deprecated stuff, candidate for deletion.
      luci_migration_host=None,

      # Relations.
      triggers=None,
      triggered_by=None,
  ):
  """Defines a generic builder.

  It runs some recipe in some requested environment, passing it a struct with
  given properties. It is launched whenever something triggers it (a poller or
  some other builder, or maybe some external actor via Buildbucket API).

  The full unique builder name (as expected by Buildbucket RPC interface) is
  a pair ("<project>", "<bucket>/<name>"), but within a single project config
  this builder can be referred to either via its bucket-scoped name (i.e.
  "<bucket>/<name>") or just via it's name alone (i.e. "<name>"), if this
  doesn't introduce ambiguities.

  The definition of what can *potentially* trigger what are defined through
  'triggers' and 'triggered_by' fields. They specify how to prepare ACLs and
  other configuration of services that execute builds.

  If builder A is defined as "triggers builder B", it means all services should
  expect A builds to trigger B builds via LUCI Scheduler's EmitTriggers RPC,
  but the actual triggering is still the responsibility of A's recipe.

  Args:
    name: name of the builder, will show up in UIs and logs.
    bucket: name of the bucket the builder belongs to.
    recipe: name of a recipe to run, see core.recipe(...) rule.

    properties: a dict with string keys with properties to pass to the recipe.
    service_account: an email of a service account to run the recipe under:
        the recipe (and various tools it calls, e.g. gsutil) will be able to
        make outbound HTTP calls that have an OAuth access token belonging to
        this service account (provided it is registered with LUCI).
    caches: a list of swarming.cache(...) objects describing Swarming named
        caches that should be present on the bot. See swarming.cache(...) doc
        for more details.
    execution_timeout: how long to wait for a running build to finish before
        forcefully aborting it and marking the build as timed out. If None, let
        Buildbucket service to decide.

    dimensions: a dict with swarming dimensions, indicating requirements for
        a bot to execute the build. Keys are strings (e.g. 'os'), and values are
        either strings (e.g. 'Linux') or swarming.dimension(...) objects. The
        latter is useful for defining dimensions with expiration, see
        swarming.dimension(...) doc for more details.
    priority: int [1-255] or None, indicating swarming task priority, lower is
        more important. If None, let Buildbucket service to decide.
    tags: a list of tags ("k:v" strings) to assign to the Swarming task that
        runs the builder. Each tag will also end up in "swarming_tag"
        Buildbucket tag, for example "swarming_tag:builder:release".
    expiration: how long to wait for a pending build to get scheduled on
        Swarming and starting executing before canceling it and marking as
        expired. If None, let Buildbucket service to decide.

    build_numbers: if True, generate monotonically increasing contiguous numbers
        for each build, unique within the builder. If None, let Buildbucket
        service to decide.
    experimental: if True, by default a new build in this builder will be marked
        as experimental. This is seen from recipes and they may behave
        differently (e.g. avoid any side-effects). If None, let Buildbucket
        service to decide.
    task_template_canary_percentage: int [0-100] or None, indicating percentage
        of builds that should use a canary swarming task template. If None, let
        Buildbucket service to decide.

    luci_migration_host: deprecated setting that was important during the
        migration from Buildbot to LUCI. Refer to Buildbucket docs for the
        meaning.

    triggers: names of builders this builder triggers.
    triggered_by: names of builders or pollers this builder is triggered by.
  """
  name = validate.string('name', name)
  bucket = validate.string('bucket', bucket)
  recipe = validate.string('recipe', recipe)

  # Node that carries the full definition of the builder.
  builder_key = keys.builder(bucket, name)
  graph.add_node(builder_key, props = {
      'name': name,
      'bucket': bucket,
      'properties': validate.str_dict('properties', properties),
      'service_account': validate.string('service_account', service_account, required=False),
      'caches': swarming.validate_caches('caches', caches),
      'execution_timeout_sec': validate.duration('execution_timeout', execution_timeout, required=False),
      'dimensions': swarming.validate_dimensions('dimensions', dimensions),
      'priority': validate.int('priority', priority, min=1, max=255, required=False),
      'tags': swarming.validate_tags('tags', tags),
      'expiration_sec': validate.duration('expiration', expiration, required=False),
      'build_numbers': validate.bool('build_numbers', build_numbers, required=False),
      'experimental': validate.bool('experimental', experimental, required=False),
      'task_template_canary_percentage': validate.int('task_template_canary_percentage', task_template_canary_percentage, min=0, max=100, required=False),
      'luci_migration_host': validate.string('luci_migration_host', luci_migration_host, required=False)
  })
  graph.add_edge(keys.bucket(bucket), builder_key)
  graph.add_edge(builder_key, keys.recipe(recipe))

  # Allow this builder to be referenced from other nodes via its bucket-scoped
  # name and via a global (perhaps ambiguous) name. See builder_ref.add(...).
  # Ambiguity is checked during the graph traversal via builder_ref.follow(...).
  builder_ref_key = builder_ref.add(builder_key)

  # Setup nodes that indicate this builder can be referenced in 'triggered_by'
  # relations (either via its bucket-scoped name or via its global name).
  triggerer_key = triggerer.add(builder_key)

  # Link to builders triggered by this builder.
  for t in validate.list('triggers', triggers):
    graph.add_edge(
        parent = triggerer_key,
        child = keys.builder_ref(validate.string('triggers', t)),
        title = 'triggers',
    )

  # And link to nodes this builder is triggered by.
  for t in validate.list('triggered_by', triggered_by):
    graph.add_edge(
        parent = keys.triggerer(validate.string('triggered_by', t)),
        child = builder_ref_key,
        title = 'triggered_by',
    )
