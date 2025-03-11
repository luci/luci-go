luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    milo = "luci-milo.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.bucket(name = "bucket")

luci.recipe(
    name = "main/recipe",
    cipd_package = "recipe/bundles/main",
)

luci.builder(
    name = "builder",
    bucket = "bucket",
    executable = "main/recipe",
)

luci.list_view(
    name = "View",
    entries = [
        "bucket/builder",  # in this current project
        "another-project:bucket/builder",  # in another project
    ],
)

luci.list_view_entry(
    list_view = "View",
    builder = "another-project:bucket/builder2",
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "bucket"
#   swarming {
#     builders {
#       name: "builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "main/recipe"
#         cipd_package: "recipe/bundles/main"
#         cipd_version: "refs/heads/main"
#       }
#     }
#   }
# }
# ===
#
# === luci-milo.cfg
# consoles {
#   id: "View"
#   name: "View"
#   builders {
#     name: "buildbucket/luci.project.bucket/builder"
#   }
#   builders {
#     name: "buildbucket/luci.another-project.bucket/builder"
#   }
#   builders {
#     name: "buildbucket/luci.another-project.bucket/builder2"
#   }
#   builder_view_only: true
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
#   name: "bucket"
# }
# ===
