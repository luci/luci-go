luci.project(
    name = "test",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm-dev.appspot.com",
)
luci.bucket(name = "dynamic", dynamic = True)

luci.task_backend(
    name = "my_task_backend",
    target = "swarming://chromium-swarm",
    config = {"key": "value"},
)

luci.task_backend(
    name = "another_backend",
    target = "another://another-host",
)

luci.dynamic_builder_template(
    bucket = "dynamic",
    executable = luci.recipe(
        name = "recipe",
        cipd_package = "cipd/package",
        cipd_version = "refs/version",
        use_bbagent = True,
    ),
    properties = {
        "prop2": ["val2", 123],
        "prop1": "val1",
    },
    allowed_property_overrides = ["prop1"],
    service_account = "builder@example.com",
    caches = [
        swarming.cache("path1"),
        swarming.cache("path2", name = "name2"),
        swarming.cache("path3", name = "name3", wait_for_warm_cache = 10 * time.minute),
    ],
    execution_timeout = 3 * time.hour,
    grace_period = 2 * time.minute,
    heartbeat_timeout = 10 * time.minute,
    priority = 80,
    expiration_timeout = time.hour,
    experiments = {
        "luci.recipes.use_python3": 100,
    },
    resultdb_settings = resultdb.settings(
        enable = True,
    ),
    test_presentation = resultdb.test_presentation(
        column_keys = ["v.gpu"],
        grouping_keys = ["status", "v.test_suite"],
    ),
    backend = "my_task_backend",
    backend_alt = "another_backend",
    contact_team_email = "nobody@email.com",
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
#       backend_alt {
#         target: "another://another-host"
#       }
#       exe {
#         cipd_package: "cipd/package"
#         cipd_version: "refs/version"
#         cmd: "luciexe"
#       }
#       properties:
#         '{'
#         '  "$recipe_engine/resultdb/test_presentation": {'
#         '    "column_keys": ['
#         '      "v.gpu"'
#         '    ],'
#         '    "grouping_keys": ['
#         '      "status",'
#         '      "v.test_suite"'
#         '    ]'
#         '  },'
#         '  "prop1": "val1",'
#         '  "prop2": ['
#         '    "val2",'
#         '    123'
#         '  ],'
#         '  "recipe": "recipe"'
#         '}'
#       allowed_property_overrides: "prop1"
#       priority: 80
#       execution_timeout_secs: 10800
#       heartbeat_timeout_secs: 600
#       expiration_secs: 3600
#       grace_period {
#         seconds: 120
#       }
#       caches {
#         name: "name2"
#         path: "path2"
#       }
#       caches {
#         name: "name3"
#         path: "path3"
#         wait_for_warm_cache_secs: 600
#       }
#       caches {
#         name: "path1"
#         path: "path1"
#       }
#       service_account: "builder@example.com"
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#       resultdb {
#         enable: true
#       }
#       contact_team_email: "nobody@email.com"
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
#   name: "dynamic"
#   bindings {
#     role: "role/buildbucket.builderServiceAccount"
#     principals: "user:builder@example.com"
#   }
# }
# ===
