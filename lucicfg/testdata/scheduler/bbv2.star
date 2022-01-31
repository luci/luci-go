lucicfg.enable_experiment("crbug.com/1182002")  # short BBv2 names + bindings

luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    scheduler = "luci-scheduler.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.bucket(name = "buck1")
luci.bucket(name = "buck2")

luci.builder(
    name = "b1",
    bucket = "buck1",
    executable = luci.recipe(name = "noop", cipd_package = "noop"),
    service_account = "b1@example.com",
    schedule = "triggered",
)

luci.builder(
    name = "b2",
    bucket = "buck1",
    executable = luci.recipe(name = "noop", cipd_package = "noop"),
    service_account = "b2@example.com",
    triggered_by = ["b1"],
)

luci.builder(
    name = "b2",
    bucket = "buck2",
    executable = luci.recipe(name = "noop", cipd_package = "noop"),
    service_account = "b3@example.com",
    triggered_by = ["b1"],
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "buck1"
#   swarming {
#     builders {
#       name: "b1"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "b1@example.com"
#     }
#     builders {
#       name: "b2"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "b2@example.com"
#     }
#   }
# }
# buckets {
#   name: "buck2"
#   swarming {
#     builders {
#       name: "b2"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "b3@example.com"
#     }
#   }
# }
# ===
#
# === luci-scheduler.cfg
# job {
#   id: "b1"
#   realm: "buck1"
#   schedule: "triggered"
#   acl_sets: "buck1"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "buck1"
#     builder: "b1"
#   }
# }
# job {
#   id: "buck1-b2"
#   realm: "buck1"
#   acls {
#     role: TRIGGERER
#     granted_to: "b1@example.com"
#   }
#   acl_sets: "buck1"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "buck1"
#     builder: "b2"
#   }
# }
# job {
#   id: "buck2-b2"
#   realm: "buck2"
#   acls {
#     role: TRIGGERER
#     granted_to: "b1@example.com"
#   }
#   acl_sets: "buck2"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "buck2"
#     builder: "b2"
#   }
# }
# acl_sets {
#   name: "buck1"
# }
# acl_sets {
#   name: "buck2"
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
#   name: "buck1"
#   bindings {
#     role: "role/buildbucket.builderServiceAccount"
#     principals: "user:b1@example.com"
#     principals: "user:b2@example.com"
#   }
#   bindings {
#     role: "role/scheduler.triggerer"
#     principals: "user:b1@example.com"
#     conditions {
#       restrict {
#         attribute: "scheduler.job.name"
#         values: "buck1-b2"
#       }
#     }
#   }
# }
# realms {
#   name: "buck2"
#   bindings {
#     role: "role/buildbucket.builderServiceAccount"
#     principals: "user:b3@example.com"
#   }
#   bindings {
#     role: "role/scheduler.triggerer"
#     principals: "user:b1@example.com"
#     conditions {
#       restrict {
#         attribute: "scheduler.job.name"
#         values: "buck2-b2"
#       }
#     }
#   }
# }
# ===
