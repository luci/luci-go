"""
The main purpose of this test file is to ensure that
`luci.builder(properties=..., test_presentation=...)` does not throw an error
when the provided `properties` is immutable. This can happen when the
`properties` were passed to/from a different execution context.
"""

# Don't set the default builder properties.
# i.e. No calls to `luci.builder.defaults.properties.set(...)`.

luci.builder.defaults.test_presentation.set(
    resultdb.test_presentation(
        column_keys = ["v.gpu"],
        grouping_keys = ["v.test_suite", "status"],
    ),
)

luci.recipe.defaults.cipd_package.set("cipd/default")
luci.recipe.defaults.cipd_version.set("refs/default")

luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    scheduler = "luci-scheduler.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.bucket(name = "ci")

shared_immutable_properties = {}

# This should not throw
# `Error in _builder: cannot insert into frozen hash table`
luci.builder(
    name = "builder1",
    bucket = "ci",
    executable = luci.recipe(
        name = "recipe",
        cipd_package = "cipd/package",
        cipd_version = "refs/version",
    ),
    properties = shared_immutable_properties,
    test_presentation = resultdb.test_presentation(
        column_keys = ["v.os"],
        grouping_keys = ["v.test_suite", "status"],
    ),
)

# This should not throw
# `Error in _builder: cannot insert into frozen hash table`
luci.builder(
    name = "builder2",
    bucket = "ci",
    executable = luci.recipe(
        name = "recipe",
        cipd_package = "cipd/package",
        cipd_version = "refs/version",
    ),
    properties = shared_immutable_properties,
    test_presentation = resultdb.test_presentation(column_keys = ["v.gpu"]),
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "ci"
#   swarming {
#     builders {
#       name: "builder1"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "recipe"
#         cipd_package: "cipd/package"
#         cipd_version: "refs/version"
#         properties_j: "$recipe_engine/resultdb/test_presentation:{\"column_keys\":[\"v.os\"],\"grouping_keys\":[\"v.test_suite\",\"status\"]}"
#       }
#     }
#     builders {
#       name: "builder2"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "recipe"
#         cipd_package: "cipd/package"
#         cipd_version: "refs/version"
#         properties_j: "$recipe_engine/resultdb/test_presentation:{\"column_keys\":[\"v.gpu\"],\"grouping_keys\":[\"status\"]}"
#       }
#     }
#   }
# }
# ===
#
# === project.cfg
# name: "project"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# realms {
#   name: "ci"
# }
# ===
