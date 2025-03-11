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

luci.recipe(
    name = "another/recipe",
    cipd_package = "recipe/bundles/another",
)

luci.dynamic_builder_template(
    bucket = "dynamic",
    executable = "main/recipe",
    service_account = "builder@example.com",
    dimensions = {
        "os": "Linux",
        "pool": "luci.ci.tester",
    },
)

luci.dynamic_builder_template(
    bucket = "dynamic",
    executable = "another/recipe",
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/dynamic_bucket_with_multiple_template.star: in <toplevel>
#   ...
# Error: dynamic bucket "dynamic" can have at most one dynamic_builder_template
