luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    notify = "luci-notify.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)
luci.bucket(name = "bucket")
luci.recipe(name = "noop", cipd_package = "noop")
luci.builder(
    name = "builder 1",
    bucket = "bucket",
    executable = "noop",
    notifies = [
        luci.notifier(
            name = "email notifier",
            on_occurrence = ["FAILURE"],
            notify_emails = ["a@example.com"],
        ),
    ],
)
luci.builder(
    name = "builder 2",
    bucket = "bucket",
    executable = "noop",
    repo = "https://repo.example.com",
    notifies = [
        luci.notifier(
            name = "blamelist notifier",
            on_occurrence = ["FAILURE"],
            notify_blamelist = True,
        ),
    ],
)
luci.builder(
    name = "builder 3",
    bucket = "bucket",
    executable = "noop",
    repo = "https://repo.example.com",
    notifies = [
        luci.notifier(
            name = "blamelist notifier with infra failures",
            on_occurrence = ["FAILURE", "INFRA_FAILURE"],
            notify_blamelist = True,
        ),
    ],
)
luci.builder(
    name = "builder 4",
    bucket = "bucket",
    executable = "noop",
    notifies = [
        luci.notifier(
            name = "notifier with regex filter",
            on_occurrence = ["FAILURE"],
            failed_step_regexp = ".*test.*",
        ),
    ],
)
luci.builder(
    name = "builder 5",
    bucket = "bucket",
    executable = "noop",
    notifies = [
        luci.notifier(
            name = "notifier with rotations",
            on_occurrence = ["FAILURE"],
            notify_rotation_urls = ["https://rota-ng.appspot.com/legacy/sheriff.json"],
        ),
    ],
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "bucket"
#   swarming {
#     builders {
#       name: "builder 1"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#     }
#     builders {
#       name: "builder 2"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#     }
#     builders {
#       name: "builder 3"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#     }
#     builders {
#       name: "builder 4"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe {
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/main"
#       }
#     }
#     builders {
#       name: "builder 5"
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
# === luci-notify.cfg
# notifiers {
#   notifications {
#     on_occurrence: FAILURE
#     email {
#       recipients: "a@example.com"
#     }
#   }
#   builders {
#     bucket: "bucket"
#     name: "builder 1"
#   }
# }
# notifiers {
#   notifications {
#     on_occurrence: FAILURE
#     notify_blamelist {}
#   }
#   builders {
#     bucket: "bucket"
#     name: "builder 2"
#     repository: "https://repo.example.com"
#   }
# }
# notifiers {
#   notifications {
#     on_occurrence: FAILURE
#     on_occurrence: INFRA_FAILURE
#     notify_blamelist {}
#   }
#   builders {
#     bucket: "bucket"
#     name: "builder 3"
#     repository: "https://repo.example.com"
#   }
# }
# notifiers {
#   notifications {
#     on_occurrence: FAILURE
#     failed_step_regexp: ".*test.*"
#   }
#   builders {
#     bucket: "bucket"
#     name: "builder 4"
#   }
# }
# notifiers {
#   notifications {
#     on_occurrence: FAILURE
#     email {
#       rotation_urls: "https://rota-ng.appspot.com/legacy/sheriff.json"
#     }
#   }
#   builders {
#     bucket: "bucket"
#     name: "builder 5"
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
#   name: "bucket"
# }
# ===
