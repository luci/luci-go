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
