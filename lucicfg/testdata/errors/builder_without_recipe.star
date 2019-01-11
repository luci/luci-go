core.project(
    name = 'project',
    buildbucket = 'cr-buildbucket.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)

core.bucket(name = 'ci')

core.builder(
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
