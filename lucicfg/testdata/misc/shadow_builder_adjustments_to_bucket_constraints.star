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
    shadow_properties = {"k": "v"},
)

luci.builder(
    name = "linux ci builder 1",
    bucket = "ci",
    executable = "main/recipe",
    service_account = "account-3@example.com",
    dimensions = {
        "os": "Linux",
        "pool": "luci.ci.tester",
        "empty_in_shadow": "true",
    },
    shadow_service_account = "shadow_builder@example.com",
    shadow_pool = "shadow_pool",
    shadow_dimensions = {
        "pool": "shadow_pool",
        "empty_in_shadow": None,
    },
)

luci.builder(
    name = "linux ci builder 2",
    bucket = "ci",
    executable = "main/recipe",
    service_account = "account-3@example.com",
    dimensions = {
        "os": "Linux",
        "pool": "luci.ci.tester",
    },
    shadow_service_account = "shadow_builder@example.com",
    shadow_dimensions = {
        "pool": "another_shadow_pool",
    },
)

# Expect configs:
#
# === cr-buildbucket.cfg
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
#         properties:
#           '{'
#           '  "k": "v"'
#           '}'
#         dimensions: "pool:shadow_pool"
#       }
#     }
#     builders {
#       name: "linux ci builder 1"
#       swarming_host: "chromium-swarm.appspot.com"
#       dimensions: "empty_in_shadow:true"
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
#         dimensions: "empty_in_shadow:"
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
#         service_account: "shadow_builder@example.com"
#         pool: "another_shadow_pool"
#         dimensions: "pool:another_shadow_pool"
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
#     pools: "another_shadow_pool"
#     pools: "shadow_pool"
#     service_accounts: "shadow_builder@example.com"
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
#     principals: "user:shadow_builder@example.com"
#   }
# }
# ===
