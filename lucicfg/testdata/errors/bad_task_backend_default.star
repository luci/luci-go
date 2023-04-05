luci.builder.defaults.backend.set("default-backend")

luci.project(
    name = "test",
    buildbucket = "cr-buildbucket.appspot.com",
)
luci.bucket(
    name = "ci",
)

luci.task_backend(
    name = "valid_task_backend",
    target = "swarming://chromium-swarm",
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
)

# Expect errors like:
#
# Traceback (most recent call last):
#   ...
# Error: luci.builder("ci/builder1") refers to undefined luci.task_backend("default-backend")
