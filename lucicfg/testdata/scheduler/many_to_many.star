luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    scheduler = "luci-scheduler.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.recipe(
    name = "noop",
    cipd_package = "noop",
)

luci.bucket(name = "ci")

luci.gitiles_poller(
    name = "p1",
    bucket = "ci",
    repo = "https://noop.com",
    triggers = ["b1", "b2", "b3"],
)
luci.gitiles_poller(
    name = "p2",
    bucket = "ci",
    repo = "https://noop.com",
    triggers = ["b1", "b2", "b3"],
)

luci.builder(
    name = "b1",
    bucket = "ci",
    executable = "noop",
    service_account = "noop1@example.com",
    triggers = ["b2", "b3"],
)
luci.builder(
    name = "b2",
    bucket = "ci",
    executable = "noop",
    service_account = "noop2@example.com",
    triggers = ["b3"],
)
luci.builder(
    name = "b3",
    bucket = "ci",
    executable = "noop",
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "ci"
#   swarming {
#     builders {
#       name: "b1"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "noop1@example.com"
#     }
#     builders {
#       name: "b2"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "noop2@example.com"
#     }
#     builders {
#       name: "b3"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#     }
#   }
# }
# ===
#
# === luci-scheduler.cfg
# job {
#   id: "b1"
#   realm: "ci"
#   acl_sets: "ci"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.ci"
#     builder: "b1"
#   }
# }
# job {
#   id: "b2"
#   realm: "ci"
#   acls {
#     role: TRIGGERER
#     granted_to: "noop1@example.com"
#   }
#   acl_sets: "ci"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.ci"
#     builder: "b2"
#   }
# }
# job {
#   id: "b3"
#   realm: "ci"
#   acls {
#     role: TRIGGERER
#     granted_to: "noop1@example.com"
#   }
#   acls {
#     role: TRIGGERER
#     granted_to: "noop2@example.com"
#   }
#   acl_sets: "ci"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.ci"
#     builder: "b3"
#   }
# }
# trigger {
#   id: "p1"
#   realm: "ci"
#   acl_sets: "ci"
#   triggers: "b1"
#   triggers: "b2"
#   triggers: "b3"
#   gitiles {
#     repo: "https://noop.com"
#     refs: "regexp:refs/heads/main"
#   }
# }
# trigger {
#   id: "p2"
#   realm: "ci"
#   acl_sets: "ci"
#   triggers: "b1"
#   triggers: "b2"
#   triggers: "b3"
#   gitiles {
#     repo: "https://noop.com"
#     refs: "regexp:refs/heads/main"
#   }
# }
# acl_sets {
#   name: "ci"
# }
# ===
#
# === project.cfg
# name: "project"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# realms {
#   name: "ci"
#   bindings {
#     role: "role/buildbucket.builderServiceAccount"
#     principals: "user:noop1@example.com"
#     principals: "user:noop2@example.com"
#   }
# }
# ===
