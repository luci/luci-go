core.project(
    name = 'infra.git',

    buildbucket = 'cr-buildbucket.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
    logdog = 'luci-logdog.appspot.com',

    acls = [
        acl.entry(acl.PROJECT_CONFIGS_READER, groups='all'),
        acl.entry(acl.BUILDBUCKET_READER, groups='all'),
        acl.entry(acl.LOGDOG_READER, groups='all'),
    ],
)

core.logdog(gs_bucket = 'chromium-luci-logdog')

# CI bucket.

core.bucket(name = 'ci')

core.gitiles_poller(
    name = 'master-poller',
    bucket = 'ci',
)

core.builder(
    name = 'linux ci builder',
    bucket = 'ci',
    triggered_by = ['master-poller'],

    properties = {
        'prop1': 'val1',
        'prop2': ['val2', 123],
    },
    service_account = 'builder@example.com',

    caches = [
        swarming.cache('path1'),
        swarming.cache('path2', name='name2'),
        swarming.cache('path3', name='name3', wait_for_warm_cache=10*time.minute),
    ],
    execution_timeout = 3 * time.hour,

    dimensions = {
        'os': 'Linux',
        'builder': 'linux ci builder',  # no auto_builder_dimension
        'prefer_if_available': [
            swarming.dimension('first-choice', expiration=5*time.minute),
            swarming.dimension('fallback'),
        ],
    },
    priority = 80,
    swarming_tags = ['tag1:val1', 'tag2:val2'],
    expiration_timeout = time.hour,
    build_numbers = True,
)


core.builder(
    name = 'generically named builder',
    bucket = 'ci',
    triggered_by = ['master-poller'],
)


# Try bucket.

core.bucket(
    name = 'try',
    acls = [
        acl.entry(acl.BUILDBUCKET_SCHEDULER, groups='infra-try-access'),
    ],
)

core.builder(
    name = 'linux try builder',
    bucket = 'try',
)
core.builder(
    name = 'generically named builder',
    bucket = 'try',
)


# Expect configs:
#
# === cr-buildbucket.cfg
# buckets: <
#   name: "ci"
#   acl_sets: "ci"
#   swarming: <
#     builders: <
#       name: "generically named builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe: <
#       >
#     >
#     builders: <
#       name: "linux ci builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       swarming_tags: "tag1:val1"
#       swarming_tags: "tag2:val2"
#       dimensions: "builder:linux ci builder"
#       dimensions: "os:Linux"
#       dimensions: "300:prefer_if_available:first-choice"
#       dimensions: "prefer_if_available:fallback"
#       recipe: <
#         properties_j: "prop1:\"val1\""
#         properties_j: "prop2:[\"val2\",123]"
#       >
#       priority: 80
#       execution_timeout_secs: 10800
#       expiration_secs: 3600
#       caches: <
#         name: "name2"
#         path: "path2"
#       >
#       caches: <
#         name: "name3"
#         path: "path3"
#         wait_for_warm_cache_secs: 600
#       >
#       caches: <
#         name: "path1"
#         path: "path1"
#       >
#       build_numbers: YES
#       service_account: "builder@example.com"
#     >
#   >
# >
# buckets: <
#   name: "try"
#   acl_sets: "try"
#   swarming: <
#     builders: <
#       name: "generically named builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe: <
#       >
#     >
#     builders: <
#       name: "linux try builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe: <
#       >
#     >
#   >
# >
# acl_sets: <
#   name: "ci"
#   acls: <
#     group: "all"
#   >
# >
# acl_sets: <
#   name: "try"
#   acls: <
#     group: "all"
#   >
#   acls: <
#     role: SCHEDULER
#     group: "infra-try-access"
#   >
# >
# ===
#
# === luci-logdog.cfg
# reader_auth_groups: "all"
# archive_gs_bucket: "chromium-luci-logdog"
# ===
#
# === project.cfg
# name: "infra.git"
# access: "group:all"
# ===
