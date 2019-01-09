core.project(
    name = 'proj',
    buildbucket = 'cr-buildbucket.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)
core.recipe(
    name = 'noop',
    cipd_package = 'noop',
)
core.bucket(name = 'b')
core.builder(
    name = 'clashing name',
    bucket = 'b',
    recipe = 'noop',
)
core.gitiles_poller(
    name = 'clashing name',
    bucket = 'b',
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/poller_builder_clash.star:16: in <toplevel>
#   ...
# Error: core.triggerer("b/clashing name") is redeclared, previous declaration:
# Traceback (most recent call last):
#   //testdata/errors/poller_builder_clash.star:11: in <toplevel>
#   ...
