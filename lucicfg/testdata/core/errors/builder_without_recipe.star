luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.bucket(name = "ci")

luci.builder(
    name = "b",
    bucket = "ci",
    service_account = "noop@example.com",
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/builder_without_recipe.star: in <toplevel>
#   ...
# Error: missing required field "executable"
