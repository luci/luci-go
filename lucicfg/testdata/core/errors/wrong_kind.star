luci.project(
    name = "proj",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.builder(
    name = "builder",
    bucket = luci.recipe(
        name = "noop",
        cipd_package = "noop",
    ),
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/wrong_kind.star: in <toplevel>
#   ...
# Error: expecting luci.bucket, got luci.executable
