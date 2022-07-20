luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
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

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "ci"
#   shadow: "ci.shadow"
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
# }
# realms {
#   name: "ci.shadow"
# }
# ===
