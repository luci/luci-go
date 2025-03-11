luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.bucket(name = "ci")

luci.recipe(
    name = "main/recipe",
    cipd_package = "recipe/bundles/main",
)

luci.builder(
    name = "b",
    bucket = "ci",
    executable = "main/recipe",
    shadow_pool = "shadow_pool",
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/builder_without_shadow_bucket_set_shadow_pool.star: in <toplevel>
#   ...
# Error: builders in bucket ci set shadow_service_account or shadow_pool, but the bucket does not have a shadow bucket
