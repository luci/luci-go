load("@proto//google/protobuf/struct.proto", struct_pb = "google.protobuf")

luci.project(
    name = "test",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm-dev.appspot.com",
)
luci.bucket(
    name = "ci",
)

luci.task_backend(
    name = "my_task_backend",
    target = "swarming://chromium-swarm",
    config = struct_pb.Struct(
        fields = {
            "key": struct_pb.Value(string_value = "val"),
        },
    ),
)
luci.builder(
    name = "builder1",
    bucket = "ci",
    executable = luci.recipe(
        name = "recipe",
        cipd_package = "cipd/package",
        cipd_version = "refs/version",
    ),
    backend_alt = "my_task_backend",
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "ci"
#   swarming {
#     builders {
#       name: "builder1"
#       backend_alt {
#         target: "swarming://chromium-swarm"
#         config_json:
#           '{'
#           '  "key": "val"'
#           '}'
#       }
#       recipe {
#         name: "recipe"
#         cipd_package: "cipd/package"
#         cipd_version: "refs/version"
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
