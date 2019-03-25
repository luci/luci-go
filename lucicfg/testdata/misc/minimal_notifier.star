luci.project(
    name = 'project',
    buildbucket = 'cr-buildbucket.appspot.com',
    notify = 'luci-notify.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)
luci.bucket(name = 'bucket')
luci.recipe(name = 'noop', cipd_package = 'noop')
luci.builder(
    name = 'builder',
    bucket = 'bucket',
    recipe = 'noop',
    notifies = [
        luci.notifier(
            name = 'email notifier',
            on_failure = True,
            notify_emails = ['a@example.com'],
        ),
    ],
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets: <
#   name: "bucket"
#   swarming: <
#     builders: <
#       name: "builder"
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
#     name: "builder"
#   >
# >
# ===
#
# === project.cfg
# name: "project"
# ===
