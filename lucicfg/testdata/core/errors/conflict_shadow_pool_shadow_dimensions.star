luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.bucket(name = "ci")

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
    shadow_dimensions = {
        "pool": "another",
    },
)
# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/conflict_shadow_pool_shadow_dimensions.star: in <toplevel>
#   ...
# Error: shadow_pool and pool dimension in shadow_dimensions should have the same value
# ...
