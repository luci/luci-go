luci.project(
    name = "test",
    buildbucket = "cr-buildbucket.appspot.com",
)
luci.bucket(
    name = "ci",
)

luci.task_backend(
    name = "invalid_task_backend1",
    target = "swarmingchromium-swarm",
    config = {"key": "value"},
)
luci.builder(
    name = "builder1",
    bucket = "ci",
    executable = luci.recipe(
        name = "recipe",
        cipd_package = "cipd/package",
        cipd_version = "refs/version",
    ),
    task_backend = "invalid_task_backend1",
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/bad_task_backend_target.star: in <toplevel>
#   ...
# Error: bad "target": invalid format for swarmingchromium-swarm
