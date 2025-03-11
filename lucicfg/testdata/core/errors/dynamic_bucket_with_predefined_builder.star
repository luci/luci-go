luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)
luci.bucket(name = "dynamic", dynamic = True)

luci.recipe(
    name = "main/recipe",
    cipd_package = "recipe/bundles/main",
)

luci.builder(
    name = "linux ci builder",
    bucket = "dynamic",
    executable = "main/recipe",
    service_account = "builder@example.com",
    dimensions = {
        "os": "Linux",
        "pool": "luci.ci.tester",
    },
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/dynamic_bucket_with_predefined_builder.star: in <toplevel>
#   ...
# Error: dynamic bucket "dynamic" must not have pre-defined builders
