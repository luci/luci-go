luci.project(
    name = "test",
    buildbucket = "cr-buildbucket.appspot.com",
)
luci.bucket(
    name = "ci",
)

luci.task_backend(
    name = "invalid_task_backend2",
    target = "swarming://chromium-swarm",
    config = "{\"key\": \"value\"}",
)
luci.builder(
    name = "builder1",
    bucket = "ci",
    executable = luci.recipe(
        name = "recipe",
        cipd_package = "cipd/package",
        cipd_version = "refs/version",
    ),
    task_backend = "invalid_task_backend2",
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/bad_task_backend_config.star: in <toplevel>
#   ...
# Error: bad config: config needs to be a dict with string keys or a proto message.
