luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.bucket(name = "ci")

luci.builder(
    name = "b",
    bucket = "ci",
    executable = luci.recipe(
        name = "main/recipe",
        cipd_package = "recipe/bundles/main",
    ),
    properties = {"a": struct(b = 123)},
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/bad_builder_props.star: in <toplevel>
#   ...
# Error in to_json: to_json: unsupported type struct
