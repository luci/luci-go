core.project(
    name = 'project',
    buildbucket = 'cr-buildbucket.appspot.com',
    scheduler = 'luci-scheduler.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)

core.recipe(
    name = 'noop',
    cipd_package = 'noop',
)

core.bucket(name = 'ci')

core.builder(
    name = 'b1',
    bucket = 'ci',
    recipe = 'noop',
    triggers = ['b2', 'b3'],
)
core.builder(
    name = 'b2',
    bucket = 'ci',
    recipe = 'noop',
)
core.builder(
    name = 'b3',
    bucket = 'ci',
    recipe = 'noop',
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/triggering_builder_no_account.star:15: in <toplevel>
#   ...
# Error: core.builder("ci/b1") needs service_account set, it triggers other builders: core.builder("ci/b2"), core.builder("ci/b3")
