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
        name = "noop",
        cipd_package = "noop",
    ),
    dimensions = {
        "key": None,
    },
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/none_dimension.star: in <toplevel>
#   ...
# Error: bad dimension "key": None value is not allowed
