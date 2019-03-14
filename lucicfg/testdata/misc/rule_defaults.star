luci.builder.defaults.properties.set({
    'base': 'base val',
    'overridden': 'original',
})
luci.builder.defaults.service_account.set('default@example.com')
luci.builder.defaults.caches.set([swarming.cache('base')])
luci.builder.defaults.execution_timeout.set(time.hour)
luci.builder.defaults.dimensions.set({
    'base': 'base val',
    'overridden': ['original 1', 'original 2'],
})
luci.builder.defaults.priority.set(30)
luci.builder.defaults.swarming_tags.set(['base:tag'])
luci.builder.defaults.expiration_timeout.set(2 * time.hour)
luci.builder.defaults.triggering_policy.set(scheduler.greedy_batching(max_batch_size=5))
luci.builder.defaults.build_numbers.set(True)
luci.builder.defaults.experimental.set(True)
luci.builder.defaults.task_template_canary_percentage.set(90)
luci.builder.defaults.luci_migration_host.set('default-migration-host')

luci.recipe.defaults.cipd_package.set('cipd/default')
luci.recipe.defaults.cipd_version.set('refs/default')


luci.project(
    name = 'project',
    buildbucket = 'cr-buildbucket.appspot.com',
    scheduler = 'luci-scheduler.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)

luci.bucket(name = 'ci')

# Pick up all defaults.
luci.builder(
    name = 'b1',
    bucket = 'ci',
    recipe = luci.recipe(name = 'recipe1'),
)

# Pick defaults and merge with provided values.
luci.builder(
    name = 'b2',
    bucket = 'ci',
    recipe = 'recipe1',
    properties = {
        'base': None,  # won't override the default
        'overridden': 'new',
        'extra': 'extra',
    },
    caches = [swarming.cache('new')],
    dimensions = {
        'base': None,  # won't override the default
        'overridden': ['new 1', 'new 2'],  # will override, not merge
    },
    swarming_tags = ['extra:tag'],
)

# Override various scalar values. In particular False, 0 and '' are treated as
# not None.
luci.builder(
    name = 'b3',
    bucket = 'ci',
    recipe = luci.recipe(
        name = 'recipe2',
        cipd_package = 'cipd/another',
        cipd_version = 'refs/another',
    ),
    service_account = 'new@example.com',
    execution_timeout = 30 * time.minute,
    priority = 1,
    expiration_timeout = 20 * time.minute,
    triggering_policy = scheduler.greedy_batching(max_batch_size=1),
    build_numbers = False,
    experimental = False,
    task_template_canary_percentage = 0,
    luci_migration_host = '',
)


# Expect configs:
#
# === cr-buildbucket.cfg
# buckets: <
#   name: "ci"
#   swarming: <
#     builders: <
#       name: "b1"
#       swarming_host: "chromium-swarm.appspot.com"
#       swarming_tags: "base:tag"
#       dimensions: "base:base val"
#       dimensions: "overridden:original 1"
#       dimensions: "overridden:original 2"
#       recipe: <
#         name: "recipe1"
#         cipd_package: "cipd/default"
#         cipd_version: "refs/default"
#         properties_j: "base:\"base val\""
#         properties_j: "overridden:\"original\""
#       >
#       priority: 30
#       execution_timeout_secs: 3600
#       expiration_secs: 7200
#       caches: <
#         name: "base"
#         path: "base"
#       >
#       build_numbers: YES
#       service_account: "default@example.com"
#       experimental: YES
#       luci_migration_host: "default-migration-host"
#       task_template_canary_percentage: <
#         value: 90
#       >
#     >
#     builders: <
#       name: "b2"
#       swarming_host: "chromium-swarm.appspot.com"
#       swarming_tags: "base:tag"
#       swarming_tags: "extra:tag"
#       dimensions: "base:base val"
#       dimensions: "overridden:new 1"
#       dimensions: "overridden:new 2"
#       recipe: <
#         name: "recipe1"
#         cipd_package: "cipd/default"
#         cipd_version: "refs/default"
#         properties_j: "base:\"base val\""
#         properties_j: "extra:\"extra\""
#         properties_j: "overridden:\"new\""
#       >
#       priority: 30
#       execution_timeout_secs: 3600
#       expiration_secs: 7200
#       caches: <
#         name: "base"
#         path: "base"
#       >
#       caches: <
#         name: "new"
#         path: "new"
#       >
#       build_numbers: YES
#       service_account: "default@example.com"
#       experimental: YES
#       luci_migration_host: "default-migration-host"
#       task_template_canary_percentage: <
#         value: 90
#       >
#     >
#     builders: <
#       name: "b3"
#       swarming_host: "chromium-swarm.appspot.com"
#       swarming_tags: "base:tag"
#       dimensions: "base:base val"
#       dimensions: "overridden:original 1"
#       dimensions: "overridden:original 2"
#       recipe: <
#         name: "recipe2"
#         cipd_package: "cipd/another"
#         cipd_version: "refs/another"
#         properties_j: "base:\"base val\""
#         properties_j: "overridden:\"original\""
#       >
#       priority: 1
#       execution_timeout_secs: 1800
#       expiration_secs: 1200
#       caches: <
#         name: "base"
#         path: "base"
#       >
#       build_numbers: NO
#       service_account: "new@example.com"
#       experimental: NO
#       task_template_canary_percentage: <
#       >
#     >
#   >
# >
# ===
#
# === luci-scheduler.cfg
# job: <
#   id: "b1"
#   acl_sets: "ci"
#   triggering_policy: <
#     kind: GREEDY_BATCHING
#     max_batch_size: 5
#   >
#   buildbucket: <
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.ci"
#     builder: "b1"
#   >
# >
# job: <
#   id: "b2"
#   acl_sets: "ci"
#   triggering_policy: <
#     kind: GREEDY_BATCHING
#     max_batch_size: 5
#   >
#   buildbucket: <
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.ci"
#     builder: "b2"
#   >
# >
# job: <
#   id: "b3"
#   acl_sets: "ci"
#   triggering_policy: <
#     kind: GREEDY_BATCHING
#     max_batch_size: 1
#   >
#   buildbucket: <
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.ci"
#     builder: "b3"
#   >
# >
# acl_sets: <
#   name: "ci"
# >
# ===
#
# === project.cfg
# name: "project"
# ===
