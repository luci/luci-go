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
            predicates = ["build.tags.get_value(\"os\")!=\"\""],
            # no custom metric fields
        ),
        buildbucket.custom_metric(
            name = "/chrome/infra/custom/builds/started",
            predicates = ["build.tags.get_value(\"os\")!=\"\""],
            extra_fields = {
                "os": "build.tags.get_value(\"os\")",
            },
        ),
    ],
)

luci.builder(
    name = "builder2",
    bucket = "ci",
    executable = luci.recipe(
        name = "recipe",
        cipd_package = "cipd/package",
        cipd_version = "refs/version",
    ),
    custom_metrics = [
        buildbucket.custom_metric(
            # It's allowed for different builders to use the same metric with
            # different predicates.
            name = "/chrome/infra/custom/builds/max_age",
            predicates = ["build.tags.get_value(\"os\")==\"\""],
        ),
    ],
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "ci"
#   swarming {
#     builders {
#       name: "builder1"
#       backend {
#         target: "swarming://chromium-swarm-default"
#         config_json:
#           '{'
#           '  "key": "value"'
#           '}'
#       }
#       recipe {
#         name: "recipe"
#         cipd_package: "cipd/package"
#         cipd_version: "refs/version"
#       }
#       custom_metric_definitions {
#         name: "/chrome/infra/custom/builds/max_age"
#         predicates: "build.tags.get_value(\"os\")!=\"\""
#       }
#       custom_metric_definitions {
#         name: "/chrome/infra/custom/builds/started"
#         predicates: "build.tags.get_value(\"os\")!=\"\""
#         extra_fields {
#           key: "os"
#           value: "build.tags.get_value(\"os\")"
#         }
#       }
#     }
#     builders {
#       name: "builder2"
#       backend {
#         target: "swarming://chromium-swarm-default"
#         config_json:
#           '{'
#           '  "key": "value"'
#           '}'
#       }
#       recipe {
#         name: "recipe"
#         cipd_package: "cipd/package"
#         cipd_version: "refs/version"
#       }
#       custom_metric_definitions {
#         name: "/chrome/infra/custom/builds/max_age"
#         predicates: "build.tags.get_value(\"os\")==\"\""
#       }
#     }
#   }
# }
# ===
#
# === project.cfg
# name: "test"
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
