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
        service_accounts = ["shadow-sa@chops-service-account.com"],
    ),
)

luci.bucket_constraints(
    bucket = "ci.shadow",
    service_accounts = ["ci-sa@chops-service-accounts.iam.gserviceaccount.com"],
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
    service_account = "builder@example.com",
    dimensions = {
        "os": "Linux",
        "pool": "luci.ci.tester",
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
#       service_account: "builder@example.com"
#     }
#   }
#   shadow: "ci.shadow"
#   constraints {
#     pools: "luci.ci.tester"
#     service_accounts: "builder@example.com"
#   }
# }
# buckets {
#   name: "ci.shadow"
#   constraints {
#     pools: "luci.chromium.ci"
#     pools: "luci.project.shadow"
#     service_accounts: "ci-sa@chops-service-accounts.iam.gserviceaccount.com"
#     service_accounts: "shadow-sa@chops-service-account.com"
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
#     principals: "user:builder@example.com"
#   }
# }
# realms {
#   name: "ci.shadow"
# }
# ===
