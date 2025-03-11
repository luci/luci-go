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

luci.builder(
    name = "b1",
    bucket = "ci",
    executable = "noop",
    service_account = "account@example.com",
)
luci.builder(
    name = "b2",
    bucket = "ci",
    executable = "noop",
    service_account = "account@example.com",
)

luci.builder(
    name = "b3",
    bucket = "ci",
    executable = "noop",
    triggered_by = ["b1", "b2"],
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
#       service_account: "account@example.com"
#     }
#     builders {
#       name: "b2"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "account@example.com"
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
#   id: "b3"
#   realm: "ci"
#   acls {
#     role: TRIGGERER
#     granted_to: "account@example.com"
#   }
#   acl_sets: "ci"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.ci"
#     builder: "b3"
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
#     principals: "user:account@example.com"
#   }
# }
# ===
