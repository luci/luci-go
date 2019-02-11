luci.project(
    name = 'project',
    buildbucket = 'cr-buildbucket.appspot.com',
    scheduler = 'luci-scheduler.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)

luci.recipe(
    name = 'noop',
    cipd_package = 'noop',
)

luci.bucket(name = 'ci')

luci.builder(
    name = 'b1',
    bucket = 'ci',
    recipe = 'noop',
    triggers = ['b2', 'b3'],
)
luci.builder(
    name = 'b2',
    bucket = 'ci',
    recipe = 'noop',
)
luci.builder(
    name = 'b3',
    bucket = 'ci',
    recipe = 'noop',
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/triggering_builder_no_account.star:15: in <toplevel>
#   ...
# Error: luci.builder("ci/b1") needs service_account set, it triggers other builders: luci.builder("ci/b2"), luci.builder("ci/b3")
