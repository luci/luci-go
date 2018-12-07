core.project(
    name = 'infra.git',
    buildbucket = 'cr-buildbucket.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
    logdog = 'luci-logdog.appspot.com',
)

core.logdog(
    gs_bucket = 'chromium-luci-logdog',
)

core.bucket(
    name = 'ci',
)

core.bucket(
    name = 'try',
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
# >
# acl_sets: <
#   name: "try"
# >
# ===
#
# === luci-logdog.cfg
# archive_gs_bucket: "chromium-luci-logdog"
# ===
#
# === project.cfg
# name: "infra.git"
# access: "group:all"
# ===
