luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.bucket(name = "ci")

luci.builder(
    name = "b",
    bucket = "ci",
    executable = luci.recipe(
        name = "main/recipe",
        cipd_package = "recipe/bundles/main",
    ),
    experiments = {
        "luci.enable_next_gen_feature": "123",
    },
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/invalid_experiments_percent.star: in <toplevel>
#   ...
# Error: bad "experiments": got string for key luci.enable_next_gen_feature, want int from 0-100
