luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    scheduler = "luci-scheduler.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.recipe(
    name = "noop",
    cipd_package = "noop",
)

luci.bucket(name = "b1")
luci.bucket(name = "b2")

# Poller<->Poller clash.
luci.gitiles_poller(
    name = "poller",
    bucket = "b1",
    repo = "https://noop.com",
)
luci.gitiles_poller(
    name = "poller",
    bucket = "b2",
    repo = "https://noop.com",
)

# Poller<->Builder clash.
luci.gitiles_poller(
    name = "poller-builder",
    bucket = "b1",
    repo = "https://noop.com",
)
luci.builder(
    name = "poller-builder",
    bucket = "b2",
    executable = "noop",
    triggered_by = ["b1/poller-builder"],
)

# Builder<->Builder clash.
luci.gitiles_poller(
    name = "some poller",
    bucket = "b1",
    repo = "https://noop.com",
)
luci.builder(
    name = "builder-builder",
    bucket = "b1",
    executable = "noop",
    triggered_by = ["some poller"],
)
luci.builder(
    name = "builder-builder",
    bucket = "b2",
    executable = "noop",
    triggered_by = ["some poller"],
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "b1"
#   swarming {
#     builders {
#       name: "builder-builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#     }
#   }
# }
# buckets {
#   name: "b2"
#   swarming {
#     builders {
#       name: "builder-builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#     }
#     builders {
#       name: "poller-builder"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#     }
#   }
# }
# ===
#
# === luci-scheduler.cfg
# job {
#   id: "b1-builder-builder"
#   realm: "b1"
#   acl_sets: "b1"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.b1"
#     builder: "builder-builder"
#   }
# }
# job {
#   id: "b2-builder-builder"
#   realm: "b2"
#   acl_sets: "b2"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.b2"
#     builder: "builder-builder"
#   }
# }
# job {
#   id: "b2-poller-builder"
#   realm: "b2"
#   acl_sets: "b2"
#   buildbucket {
#     server: "cr-buildbucket.appspot.com"
#     bucket: "luci.project.b2"
#     builder: "poller-builder"
#   }
# }
# trigger {
#   id: "b1-poller"
#   realm: "b1"
#   acl_sets: "b1"
#   gitiles {
#     repo: "https://noop.com"
#     refs: "regexp:refs/heads/main"
#   }
# }
# trigger {
#   id: "b1-poller-builder"
#   realm: "b1"
#   acl_sets: "b1"
#   triggers: "b2-poller-builder"
#   gitiles {
#     repo: "https://noop.com"
#     refs: "regexp:refs/heads/main"
#   }
# }
# trigger {
#   id: "b2-poller"
#   realm: "b2"
#   acl_sets: "b2"
#   gitiles {
#     repo: "https://noop.com"
#     refs: "regexp:refs/heads/main"
#   }
# }
# trigger {
#   id: "some poller"
#   realm: "b1"
#   acl_sets: "b1"
#   triggers: "b1-builder-builder"
#   triggers: "b2-builder-builder"
#   gitiles {
#     repo: "https://noop.com"
#     refs: "regexp:refs/heads/main"
#   }
# }
# acl_sets {
#   name: "b1"
# }
# acl_sets {
#   name: "b2"
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
#   name: "b1"
# }
# realms {
#   name: "b2"
# }
# ===
