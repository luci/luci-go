luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    notify = "luci-notify.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)
luci.notify(tree_closing_enabled = True)
luci.bucket(name = "bucket")
luci.builder(
    name = "builder 1",
    bucket = "bucket",
    executable = luci.recipe(name = "noop", cipd_package = "noop"),
    notifies = [
        luci.notifier(
            name = "email notifier",
            on_occurrence = ["FAILURE"],
            notify_emails = ["a@example.com"],
        ),
        "tree closer",
    ],
)
luci.builder(
    name = "builder 2",
    bucket = "bucket",
    executable = "noop",
    repo = "https://repo.example.com",
)
luci.tree_closer(
    name = "tree closer",
    tree_status_host = "some-tree.example.com",
    failed_step_regexp = "failed-step-regexp",
    failed_step_regexp_exclude = ["regex1", "or maybe regex 2"],
    template = luci.notifier_template(
        name = "tree_status",
        body = "boom\n",
    ),
    notified_by = ["builder 2"],
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
#   tree_closers {
#     tree_status_host: "some-tree.example.com"
#     failed_step_regexp: "failed-step-regexp"
#     failed_step_regexp_exclude: "regex1|or maybe regex 2"
#     template: "tree_status"
#   }
# }
# notifiers {
#   builders {
#     bucket: "bucket"
#     name: "builder 2"
#     repository: "https://repo.example.com"
#   }
#   tree_closers {
#     tree_status_host: "some-tree.example.com"
#     failed_step_regexp: "failed-step-regexp"
#     failed_step_regexp_exclude: "regex1|or maybe regex 2"
#     template: "tree_status"
#   }
# }
# tree_closing_enabled: true
# ===
#
# === luci-notify/email-templates/tree_status.template
# boom
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
