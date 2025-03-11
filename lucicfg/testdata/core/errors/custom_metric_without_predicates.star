luci.project(
    name = "test",
    buildbucket = "cr-buildbucket.appspot.com",
)
luci.bucket(
    name = "ci",
)

luci.task_backend(
    name = "deafult-backend",
    target = "swarming://chromium-swarm-default",
    config = {"key": "value"},
)

luci.builder.defaults.backend.set("deafult-backend")

luci.builder(
    name = "builder1",
    bucket = "ci",
    executable = luci.recipe(
        name = "recipe",
        cipd_package = "cipd/package",
        cipd_version = "refs/version",
    ),
    custom_metrics = [
        buildbucket.custom_metric(
            name = "/chrome/infra/custom/builds/max_age",
            predicates = None,
        ),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/custom_metric_without_predicates.star: in <toplevel>
#   ...
# Error: missing required field "predicates"
