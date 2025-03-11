luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.bucket(name = "ci")

luci.builder(
    name = "b",
    bucket = "ci",
    properties = {
        "hello": "world",
    },
    allowed_property_overrides = ["other_prop"],
    executable = luci.recipe(
        name = "morp",
        cipd_package = "meep",
        cipd_version = "yes",
    ),
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/property_overrides.star: in <toplevel>
#   ...
# Error: "other_prop" listed in allowed_property_overrides but not in properties
