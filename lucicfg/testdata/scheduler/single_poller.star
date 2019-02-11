core.project(
    name = 'project',
    buildbucket = 'cr-buildbucket.appspot.com',
    scheduler = 'luci-scheduler.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)

core.bucket(name = 'ci')

# This poller is still defined even though it doesn't trigger anything.
core.gitiles_poller(
    name = 'poller',
    repo = 'https://noop.com',
    bucket = 'ci',
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets: <
#   name: "ci"
#   swarming: <
#   >
# >
# ===
#
# === luci-scheduler.cfg
# trigger: <
#   id: "poller"
#   acl_sets: "ci"
#   gitiles: <
#     repo: "https://noop.com"
#     refs: "refs/heads/master"
#   >
# >
# acl_sets: <
#   name: "ci"
# >
# ===
#
# === project.cfg
# name: "project"
# ===
