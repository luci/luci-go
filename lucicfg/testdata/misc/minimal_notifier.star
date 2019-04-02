luci.project(
    name = 'project',
    buildbucket = 'cr-buildbucket.appspot.com',
    notify = 'luci-notify.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)
luci.bucket(name = 'bucket')
luci.recipe(name = 'noop', cipd_package = 'noop')
luci.builder(
    name = 'builder 1',
    bucket = 'bucket',
    executable = 'noop',
    notifies = [
        luci.notifier(
            name = 'email notifier',
            on_failure = True,
            notify_emails = ['a@example.com'],
        ),
    ],
)
luci.builder(
    name = 'builder 2',
    bucket = 'bucket',
    executable = 'noop',
    repo = 'https://repo.example.com',
    notifies = [
        luci.notifier(
            name = 'blamelist notifier',
            on_failure = True,
            notify_blamelist = True,
        ),
    ],
)

# === cr-buildbucket.cfg
# buckets: <
#   name: "bucket"
#   swarming: <
#     builders: <
#       name: "builder 1"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe: <
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/master"
#       >
#     >
#     builders: <
#       name: "builder 2"
#       swarming_host: "chromium-swarm.appspot.com"
#       recipe: <
#         name: "noop"
#         cipd_package: "noop"
#         cipd_version: "refs/heads/master"
#       >
#     >
#   >
# >
# ===
#
# === luci-notify.cfg
# notifiers: <
#   notifications: <
#     on_failure: true
#     email: <
#       recipients: "a@example.com"
#     >
#   >
#   builders: <
#     bucket: "bucket"
#     name: "builder 1"
#   >
# >
# notifiers: <
#   notifications: <
#     on_failure: true
#     notify_blamelist: <
#     >
#   >
#   builders: <
#     bucket: "bucket"
#     name: "builder 2"
#     repository: "https://repo.example.com"
#   >
# >
# ===
#
# === project.cfg
# name: "project"
# ===
