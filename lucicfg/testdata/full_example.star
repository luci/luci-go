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

# CI builders.

core.bucket(name = 'ci')

core.builder(
    name = 'linux ci builder',
    bucket = 'ci',
)
core.builder(
    name = 'shared builder',
    bucket = 'ci',
)

# Try builders.

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
    name = 'shared builder',
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
#       name: "linux ci builder"
#       swarming_host: "chromium-swarm.appspot.com"
#     >
#     builders: <
#       name: "shared builder"
#       swarming_host: "chromium-swarm.appspot.com"
#     >
#   >
# >
# buckets: <
#   name: "try"
#   acl_sets: "try"
#   swarming: <
#     builders: <
#       name: "linux try builder"
#       swarming_host: "chromium-swarm.appspot.com"
#     >
#     builders: <
#       name: "shared builder"
#       swarming_host: "chromium-swarm.appspot.com"
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
