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

core.bucket(name = 'b1')
core.bucket(name = 'b2')

core.gitiles_poller(
    name = 'p',
    bucket = 'b1',
)
core.gitiles_poller(
    name = 'p',
    bucket = 'b2',
)

core.builder(
    name = 'b1-p',
    bucket = 'b1',
    recipe = 'noop',
    triggered_by = ['b1/p'] # need to be triggered but something to get job{...}
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/scheduler_disambiguation_fail.star:25: in <toplevel>
#   ...
# Error: core.builder("b1/b1-p") and core.gitiles_poller("b1/p") cause ambiguities in the scheduler config file, pick names that don't start with a bucket name
