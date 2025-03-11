luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    milo = "luci-milo.appspot.com",
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
)

luci.list_view_entry("b")

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/list_view_orphan_entry.star: in <toplevel>
#   ...
# Error: luci.list_view_entry("b") is not added to any view, either remove or comment it out
