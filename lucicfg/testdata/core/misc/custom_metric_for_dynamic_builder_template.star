luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)
luci.bucket(name = "dynamic", dynamic = True)

luci.task_backend(
    name = "my_task_backend",
    target = "swarming://chromium-swarm",
    config = {"key": "value"},
)

luci.dynamic_builder_template(
    bucket = "dynamic",
    backend = "my_task_backend",
    custom_metrics = [
        buildbucket.custom_metric(
            name = "/chrome/infra/custom/builds/max_age",
            predicates = ["build.tags.get_value(\"os\")!=\"\""],
        ),
    ],
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "dynamic"
#   dynamic_builder_template {
#     template {
#       backend {
#         target: "swarming://chromium-swarm"
#         config_json:
#           '{'
#           '  "key": "value"'
#           '}'
#       }
#       custom_metric_definitions {
#         name: "/chrome/infra/custom/builds/max_age"
#         predicates: "build.tags.get_value(\"os\")!=\"\""
#       }
#     }
#   }
# }
# ===
#
# === project.cfg
# name: "zzz"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# realms {
#   name: "dynamic"
# }
# ===
