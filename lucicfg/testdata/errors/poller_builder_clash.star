core.project(
    name = 'proj',
    buildbucket = 'cr-buildbucket.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
    scheduler = 'luci-scheduler.appspot.com',
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
    repo = 'https://noop.com',
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/poller_builder_clash.star:17: in <toplevel>
#   ...
# Error: core.triggerer("b/clashing name") is redeclared, previous declaration:
# Traceback (most recent call last):
#   //testdata/errors/poller_builder_clash.star:12: in <toplevel>
#   ...
