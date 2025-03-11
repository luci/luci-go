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

luci.list_view(
    name = "View",
    entries = [
        # Allowed forms.
        "b1",
        luci.list_view_entry("b2"),
        luci.builder(
            name = "b3",
            bucket = "ci",
            executable = "main/recipe",
        ),
        luci.list_view_entry(luci.builder(
            name = "b4",
            bucket = "ci",
            executable = "main/recipe",
        )),
        # Wrong kind.
        luci.recipe(
            name = "recipe",
            cipd_package = "recipe/bundles/main",
        ),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/list_view_wrong_entry.star: in <toplevel>
#   ...
# Error: expecting luci.list_view_entry, got luci.executable
