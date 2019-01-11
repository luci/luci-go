core.project(
    name = 'project',
    buildbucket = 'cr-buildbucket.appspot.com',
    scheduler = 'luci-scheduler.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)

core.recipe(
    name = 'noop',
    cipd_package = 'noop',
)

core.bucket(name = 'ci')

core.gitiles_poller(
    name = 'poller',
    bucket = 'ci',
    triggers = ['builder'],
)

core.builder(
    name = 'builder',
    bucket = 'ci',
    recipe = 'noop',
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets: <
#   name: "ci"
#   acl_sets: "ci"
#   swarming: <
#     builders: <
#       name: "builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe: <
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/master"
#       >
#     >
#   >
# >
# acl_sets: <
#   name: "ci"
# >
# ===
#
# === luci-scheduler.cfg
# job: <
#   id: "builder"
#   acl_sets: "ci"
#   buildbucket: <
#     server: "cr-buildbucket.appspot.com"
#     bucket: "ci"
#     builder: "builder"
#   >
# >
# trigger: <
#   id: "poller"
#   acl_sets: "ci"
#   triggers: "builder"
# >
# acl_sets: <
#   name: "ci"
# >
# ===
#
# === project.cfg
# name: "project"
# ===
