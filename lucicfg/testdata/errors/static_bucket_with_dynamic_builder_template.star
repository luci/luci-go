luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)
luci.bucket(name = "ci")

luci.recipe(
    name = "main/recipe",
    cipd_package = "recipe/bundles/main",
)

luci.dynamic_builder_template(
    bucket = "ci",
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
#   //testdata/errors/static_bucket_with_dynamic_builder_template.star: in <toplevel>
#   ...
# Error: bucket "ci" must not have dynamic_builder_template
