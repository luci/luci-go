luci.builder.defaults.properties.set({
    "base": "base val",
    "overridden": "original",
})
luci.builder.defaults.allowed_property_overrides.set(["overridden"])
luci.builder.defaults.service_account.set("default@example.com")
luci.builder.defaults.caches.set([swarming.cache("base")])
luci.builder.defaults.execution_timeout.set(time.hour)
luci.builder.defaults.dimensions.set({
    "base": "base val",
    "overridden": ["original 1", "original 2"],
})
luci.builder.defaults.priority.set(30)
luci.builder.defaults.swarming_tags.set(["base:tag"])
luci.builder.defaults.expiration_timeout.set(2 * time.hour)
luci.builder.defaults.triggering_policy.set(scheduler.greedy_batching(max_batch_size = 5))
luci.builder.defaults.build_numbers.set(True)
luci.builder.defaults.experimental.set(True)
luci.builder.defaults.experiments.set({
    "def-exp-3": 30,
    "def-exp-2": 20,
    "def-exp-1": 10,
})
luci.builder.defaults.task_template_canary_percentage.set(90)
luci.builder.defaults.test_presentation.set(resultdb.test_presentation(column_keys = ["v.gpu"], grouping_keys = ["v.test_suite", "status"]))

luci.recipe.defaults.cipd_package.set("cipd/default")
luci.recipe.defaults.cipd_version.set("refs/default")
luci.recipe.defaults.use_bbagent.set(True)
luci.recipe.defaults.use_python3.set(True)

luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    scheduler = "luci-scheduler.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.bucket(name = "ci")

# Pick up all defaults.
luci.builder(
    name = "b1",
    bucket = "ci",
    executable = luci.recipe(name = "recipe1"),
)

# Pick defaults and merge with provided values.
luci.builder(
    name = "b2",
    bucket = "ci",
    executable = "recipe1",
    properties = {
        "base": None,  # won't override the default
        "overridden": "new",
        "extra": "extra",
    },
    allowed_property_overrides = ["extra"],
    caches = [swarming.cache("new")],
    dimensions = {
        "base": None,  # won't override the default
        "overridden": ["new 1", "new 2"],  # will override, not merge
    },
    swarming_tags = ["extra:tag"],
    experiments = {
        "def-exp-1": None,  # won't override the default
        "def-exp-2": 0,  # will override the default
        "builder-exp": 100,
    },
    test_presentation = resultdb.test_presentation(column_keys = ["v.os"], grouping_keys = ["v.test_suite", "status"]),
)

# Override various scalar values. In particular False, 0 and '' are treated as
# not None.
luci.builder(
    name = "b3",
    bucket = "ci",
    executable = luci.recipe(
        name = "recipe2",
        cipd_package = "cipd/another",
        cipd_version = "refs/another",
    ),
    service_account = "new@example.com",
    execution_timeout = 30 * time.minute,
    grace_period = 2 * time.minute,
    priority = 1,
    expiration_timeout = 20 * time.minute,
    wait_for_capacity = True,
    retriable = False,
    triggering_policy = scheduler.greedy_batching(max_batch_size = 1),
    build_numbers = False,
    experimental = False,
    task_template_canary_percentage = 0,
)

# Override test_presentation back to system default.
luci.builder(
    name = "b4",
    bucket = "ci",
    executable = luci.recipe(name = "recipe1"),
    test_presentation = resultdb.test_presentation(),
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "ci"
#   swarming {
#     builders {
#       name: "b1"
#       swarming_host: "chromium-swarm.appspot.com"
#       swarming_tags: "base:tag"
#       dimensions: "base:base val"
#       dimensions: "overridden:original 1"
#       dimensions: "overridden:original 2"
#       exe {
#         cipd_package: "cipd/default"
#         cipd_version: "refs/default"
#         cmd: "luciexe"
#       }
#       properties:
#         '{'
#         '  "$recipe_engine/resultdb/test_presentation": {'
#         '    "column_keys": ['
#         '      "v.gpu"'
#         '    ],'
#         '    "grouping_keys": ['
#         '      "v.test_suite",'
#         '      "status"'
#         '    ]'
#         '  },'
#         '  "base": "base val",'
#         '  "overridden": "original",'
#         '  "recipe": "recipe1"'
#         '}'
#       allowed_property_overrides: "overridden"
#       priority: 30
#       execution_timeout_secs: 3600
#       expiration_secs: 7200
#       caches {
#         name: "base"
#         path: "base"
#       }
#       build_numbers: YES
#       service_account: "default@example.com"
#       experimental: YES
#       task_template_canary_percentage {
#         value: 90
#       }
#       experiments {
#         key: "def-exp-1"
#         value: 10
#       }
#       experiments {
#         key: "def-exp-2"
#         value: 20
#       }
#       experiments {
#         key: "def-exp-3"
#         value: 30
#       }
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#     builders {
#       name: "b2"
#       swarming_host: "chromium-swarm.appspot.com"
#       swarming_tags: "base:tag"
#       swarming_tags: "extra:tag"
#       dimensions: "base:base val"
#       dimensions: "overridden:new 1"
#       dimensions: "overridden:new 2"
#       exe {
#         cipd_package: "cipd/default"
#         cipd_version: "refs/default"
#         cmd: "luciexe"
#       }
#       properties:
#         '{'
#         '  "$recipe_engine/resultdb/test_presentation": {'
#         '    "column_keys": ['
#         '      "v.os"'
#         '    ],'
#         '    "grouping_keys": ['
#         '      "v.test_suite",'
#         '      "status"'
#         '    ]'
#         '  },'
#         '  "base": "base val",'
#         '  "extra": "extra",'
#         '  "overridden": "new",'
#         '  "recipe": "recipe1"'
#         '}'
#       allowed_property_overrides: "extra"
#       allowed_property_overrides: "overridden"
#       priority: 30
#       execution_timeout_secs: 3600
#       expiration_secs: 7200
#       caches {
#         name: "base"
#         path: "base"
#       }
#       caches {
#         name: "new"
#         path: "new"
#       }
#       build_numbers: YES
#       service_account: "default@example.com"
#       experimental: YES
#       task_template_canary_percentage {
#         value: 90
#       }
#       experiments {
#         key: "builder-exp"
#         value: 100
#       }
#       experiments {
#         key: "def-exp-1"
#         value: 10
#       }
#       experiments {
#         key: "def-exp-2"
#         value: 0
#       }
#       experiments {
#         key: "def-exp-3"
#         value: 30
#       }
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#     builders {
#       name: "b3"
#       swarming_host: "chromium-swarm.appspot.com"
#       swarming_tags: "base:tag"
#       dimensions: "base:base val"
#       dimensions: "overridden:original 1"
#       dimensions: "overridden:original 2"
#       exe {
#         cipd_package: "cipd/another"
#         cipd_version: "refs/another"
#         cmd: "luciexe"
#       }
#       properties:
#         '{'
#         '  "$recipe_engine/resultdb/test_presentation": {'
#         '    "column_keys": ['
#         '      "v.gpu"'
#         '    ],'
#         '    "grouping_keys": ['
#         '      "v.test_suite",'
#         '      "status"'
#         '    ]'
#         '  },'
#         '  "base": "base val",'
#         '  "overridden": "original",'
#         '  "recipe": "recipe2"'
#         '}'
#       allowed_property_overrides: "overridden"
#       priority: 1
#       execution_timeout_secs: 1800
#       expiration_secs: 1200
#       grace_period {
#         seconds: 120
#       }
#       wait_for_capacity: YES
#       caches {
#         name: "base"
#         path: "base"
#       }
#       build_numbers: NO
#       service_account: "new@example.com"
#       experimental: NO
#       task_template_canary_percentage {}
#       experiments {
#         key: "def-exp-1"
#         value: 10
#       }
#       experiments {
#         key: "def-exp-2"
#         value: 20
#       }
#       experiments {
#         key: "def-exp-3"
#         value: 30
#       }
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#       retriable: NO
#     }
#     builders {
#       name: "b4"
#       swarming_host: "chromium-swarm.appspot.com"
#       swarming_tags: "base:tag"
#       dimensions: "base:base val"
#       dimensions: "overridden:original 1"
#       dimensions: "overridden:original 2"
#       exe {
#         cipd_package: "cipd/default"
#         cipd_version: "refs/default"
#         cmd: "luciexe"
#       }
#       properties:
#         '{'
#         '  "base": "base val",'
#         '  "overridden": "original",'
#         '  "recipe": "recipe1"'
#         '}'
#       allowed_property_overrides: "overridden"
#       priority: 30
#       execution_timeout_secs: 3600
#       expiration_secs: 7200
#       caches {
#         name: "base"
#         path: "base"
#       }
#       build_numbers: YES
#       service_account: "default@example.com"
#       experimental: YES
#       task_template_canary_percentage {
#         value: 90
#       }
#       experiments {
#         key: "def-exp-1"
#         value: 10
#       }
#       experiments {
#         key: "def-exp-2"
#         value: 20
#       }
#       experiments {
#         key: "def-exp-3"
#         value: 30
#       }
#       experiments {
#         key: "luci.recipes.use_python3"
#         value: 100
#       }
#     }
#   }
# }
# ===
#
# === luci-scheduler.cfg
# job {
#   id: "b1"
#   realm: "ci"
#   acl_sets: "ci"
#   triggering_policy {
#     kind: GREEDY_BATCHING
#     max_batch_size: 5
#   }
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.ci"
#     builder: "b1"
#   }
# }
# job {
#   id: "b2"
#   realm: "ci"
#   acl_sets: "ci"
#   triggering_policy {
#     kind: GREEDY_BATCHING
#     max_batch_size: 5
#   }
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.ci"
#     builder: "b2"
#   }
# }
# job {
#   id: "b3"
#   realm: "ci"
#   acl_sets: "ci"
#   triggering_policy {
#     kind: GREEDY_BATCHING
#     max_batch_size: 1
#   }
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.ci"
#     builder: "b3"
#   }
# }
# job {
#   id: "b4"
#   realm: "ci"
#   acl_sets: "ci"
#   triggering_policy {
#     kind: GREEDY_BATCHING
#     max_batch_size: 5
#   }
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.ci"
#     builder: "b4"
#   }
# }
# acl_sets {
#   name: "ci"
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
#   bindings {
#     role: "role/buildbucket.builderServiceAccount"
#     principals: "user:default@example.com"
#     principals: "user:new@example.com"
#   }
# }
# ===
