luci.project(
    name = 'project',
    buildbucket = 'cr-buildbucket.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)

luci.bucket(name = 'ci')

luci.builder(
    name = 'b',
    bucket = 'ci',
    service_account = 'noop@example.com',
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/builder_without_recipe.star:9: in <toplevel>
#   ...
# Error: missing required field "recipe"
