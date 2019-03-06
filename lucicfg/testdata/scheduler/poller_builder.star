luci.project(
    name = 'project',
    buildbucket = 'cr-buildbucket.appspot.com',
    scheduler = 'luci-scheduler.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)

luci.recipe(
    name = 'noop',
    cipd_package = 'noop',
)

luci.bucket(name = 'ci')

luci.gitiles_poller(
    name = 'poller',
    bucket = 'ci',
    repo = 'https://noop.com',
    triggers = ['builder'],
)

luci.builder(
    name = 'builder',
    bucket = 'ci',
    recipe = 'noop',
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets: <
#   name: "ci"
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
# ===
#
# === luci-scheduler.cfg
# job: <
#   id: "builder"
#   acl_sets: "ci"
#   buildbucket: <
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.ci"
#     builder: "builder"
#   >
# >
# trigger: <
#   id: "poller"
#   acl_sets: "ci"
#   triggers: "builder"
#   gitiles: <
#     repo: "https://noop.com"
#     refs: "regexp:refs/heads/master"
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
