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

core.logdog(
    gs_bucket = 'chromium-luci-logdog',
)

core.bucket(
    name = 'ci',
)

core.bucket(
    name = 'try',

    acls = [
        acl.entry(acl.BUILDBUCKET_SCHEDULER, groups='infra-try-access'),
    ],
)


# Expect configs:
#
# === cr-buildbucket.cfg
# buckets: <
#   name: "ci"
#   acl_sets: "ci"
#   swarming: <
#     hostname: "chromium-swarm.appspot.com"
#   >
# >
# buckets: <
#   name: "try"
#   acl_sets: "try"
#   swarming: <
#     hostname: "chromium-swarm.appspot.com"
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
