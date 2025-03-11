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
    allowed_property_overrides = ["thing*"],
    executable = luci.recipe(
        name = "morp",
        cipd_package = "meep",
        cipd_version = "yes",
    ),
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/property_overrides_wildcard.star: in <toplevel>
#   ...
# Error: allowed_property_overrides does not support wildcards: "thing*"
