luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    scheduler = "luci-scheduler.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.recipe(
    name = "noop",
    cipd_package = "noop",
)

luci.bucket(name = "b1")
luci.bucket(name = "b2")

luci.gitiles_poller(
    name = "p",
    bucket = "b1",
    repo = "https://noop.com",
)
luci.gitiles_poller(
    name = "p",
    bucket = "b2",
    repo = "https://noop.com",
)

luci.builder(
    name = "b1-p",
    bucket = "b1",
    executable = "noop",
    triggered_by = ["b1/p"],  # need to be triggered but something to get job{...}
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/scheduler_disambiguation_fail.star: in <toplevel>
#   ...
# Error: luci.builder("b1/b1-p") and luci.gitiles_poller("b1/p") cause ambiguities in the scheduler config file, pick names that don't start with a bucket name
