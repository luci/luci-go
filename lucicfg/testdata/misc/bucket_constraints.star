lucicfg.enable_experiment("crbug.com/1338648")

luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)
luci.bucket(name = "ci")
luci.bucket(
    name = "ci.shadow",
    shadows = "ci",
    constraints = luci.bucket_constraints(
        pools = ["luci.project.shadow"],
        service_accounts = ["account-1@example.com"],
    ),
)

luci.bucket_constraints(
    bucket = "ci.shadow",
    service_accounts = ["account-2@example.com"],
)
luci.bucket_constraints(
    bucket = "ci.shadow",
    pools = ["luci.chromium.ci", "luci.project.shadow"],
)

luci.recipe(
    name = "main/recipe",
    cipd_package = "recipe/bundles/main",
)

luci.builder(
    name = "linux ci builder",
    bucket = "ci",
    executable = "main/recipe",
    service_account = "account-3@example.com",
    dimensions = {
        "os": "Linux",
        "pool": "luci.ci.tester",
    },
    shadow_service_account = "shadow_builder@example.com",
    shadow_pool = "shadow_pool",
)

luci.builder(
    name = "linux ci builder 1",
    bucket = "ci",
    executable = "main/recipe",
    service_account = "account-3@example.com",
    dimensions = {
        "os": "Linux",
        "pool": "luci.ci.tester",
    },
    shadow_service_account = "account-1@example.com",
    shadow_pool = "shadow_pool",
)

#
luci.builder(
    name = "linux ci builder 2",
    bucket = "ci",
    executable = "main/recipe",
    service_account = "account-3@example.com",
    dimensions = {
        "os": "Linux",
        "pool": "luci.ci.tester",
    },
    shadow_service_account = "a@example.com",
    shadow_pool = "shadow_pool",
)

luci.bucket_constraints(
    bucket = luci.bucket(name = "another"),
    service_accounts = ["account-4@example.com"],
)

# Service accounts are actually optional.
luci.bucket(
    name = "one-more",
    constraints = luci.bucket_constraints(pools = ["luci.chromium.ci"]),
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "another"
#   constraints {
#     service_accounts: "account-4@example.com"
#   }
# }
# buckets {
#   name: "ci"
#   swarming {
#     builders {
#       name: "linux ci builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       dimensions: "os:Linux"
#       dimensions: "pool:luci.ci.tester"
#       recipe {
#         name: "main/recipe"
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "account-3@example.com"
#       shadow_builder_adjustments {
#         service_account: "shadow_builder@example.com"
#         pool: "shadow_pool"
#         dimensions: "pool:shadow_pool"
#       }
#     }
#     builders {
#       name: "linux ci builder 1"
#       swarming_host: "chromium-swarm.appspot.com"
#       dimensions: "os:Linux"
#       dimensions: "pool:luci.ci.tester"
#       recipe {
#         name: "main/recipe"
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "account-3@example.com"
#       shadow_builder_adjustments {
#         service_account: "account-1@example.com"
#         pool: "shadow_pool"
#         dimensions: "pool:shadow_pool"
#       }
#     }
#     builders {
#       name: "linux ci builder 2"
#       swarming_host: "chromium-swarm.appspot.com"
#       dimensions: "os:Linux"
#       dimensions: "pool:luci.ci.tester"
#       recipe {
#         name: "main/recipe"
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#       }
#       service_account: "account-3@example.com"
#       shadow_builder_adjustments {
#         service_account: "a@example.com"
#         pool: "shadow_pool"
#         dimensions: "pool:shadow_pool"
#       }
#     }
#   }
#   shadow: "ci.shadow"
#   constraints {
#     pools: "luci.ci.tester"
#     service_accounts: "account-3@example.com"
#   }
# }
# buckets {
#   name: "ci.shadow"
#   constraints {
#     pools: "luci.chromium.ci"
#     pools: "luci.project.shadow"
#     pools: "shadow_pool"
#     service_accounts: "a@example.com"
#     service_accounts: "account-1@example.com"
#     service_accounts: "account-2@example.com"
#     service_accounts: "shadow_builder@example.com"
#   }
# }
# buckets {
#   name: "one-more"
#   constraints {
#     pools: "luci.chromium.ci"
#   }
# }
# ===
#
# === project.cfg
# name: "zzz"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# realms {
#   name: "another"
#   bindings {
#     role: "role/buildbucket.builderServiceAccount"
#     principals: "user:account-4@example.com"
#   }
# }
# realms {
#   name: "ci"
#   bindings {
#     role: "role/buildbucket.builderServiceAccount"
#     principals: "user:account-3@example.com"
#   }
# }
# realms {
#   name: "ci.shadow"
#   bindings {
#     role: "role/buildbucket.builderServiceAccount"
#     principals: "user:a@example.com"
#     principals: "user:account-1@example.com"
#     principals: "user:account-2@example.com"
#     principals: "user:shadow_builder@example.com"
#   }
# }
# realms {
#   name: "one-more"
# }
# ===
