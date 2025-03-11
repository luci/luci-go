luci.project(
    name = "proj",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
    scheduler = "luci-scheduler.appspot.com",
)
luci.recipe(
    name = "noop",
    cipd_package = "noop",
)
luci.bucket(name = "b")
luci.builder(
    name = "clashing name",
    bucket = "b",
    executable = "noop",
)
luci.gitiles_poller(
    name = "clashing name",
    bucket = "b",
    repo = "https://noop.com",
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/poller_builder_clash.star: in <toplevel>
#   ...
# Error in add_node: luci.triggerer("b/clashing name") is redeclared, previous declaration:
# Traceback (most recent call last):
#   //errors/poller_builder_clash.star: in <toplevel>
#   ...
