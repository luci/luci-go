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

core.builder(
    name = 'b1',
    bucket = 'ci',
    recipe = 'noop',
    service_account = 'account@example.com',
)
core.builder(
    name = 'b2',
    bucket = 'ci',
    recipe = 'noop',
    service_account = 'account@example.com',
)

core.builder(
    name = 'b3',
    bucket = 'ci',
    recipe = 'noop',
    triggered_by = ['b1', 'b2'],
)

# Expect configs:
#
#
# === cr-buildbucket.cfg
# buckets: <
#   name: "ci"
#   acl_sets: "ci"
#   swarming: <
#     builders: <
#       name: "b1"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe: <
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/master"
#       >
#       service_account: "account@example.com"
#     >
#     builders: <
#       name: "b2"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe: <
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/master"
#       >
#       service_account: "account@example.com"
#     >
#     builders: <
#       name: "b3"
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
#   id: "b3"
#   acls: <
#     role: TRIGGERER
#     granted_to: "account@example.com"
#   >
#   acl_sets: "ci"
#   buildbucket: <
#     server: "cr-buildbucket.appspot.com"
#     bucket: "ci"
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
