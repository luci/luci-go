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
    name = "b1",
    bucket = "ci",
    executable = "main/recipe",
)

luci.builder(
    name = "b2",
    bucket = "ci",
    executable = "main/recipe",
)

b2_entry = luci.list_view_entry("b2")

luci.list_view(
    name = "View 1",
    entries = [
        "b1",
        b2_entry,
    ],
)

luci.list_view(
    name = "View 2",
    entries = [
        "b1",
        b2_entry,
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/list_view_multiparent_entry.star: in <toplevel>
#   ...
# Error: luci.list_view_entry("b2") is added to multiple views: luci.list_view("View 1"), luci.list_view("View 2")
