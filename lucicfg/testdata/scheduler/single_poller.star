luci.project(
    name = "project",
    buildbucket = "cr-buildbucket.appspot.com",
    scheduler = "luci-scheduler.appspot.com",
)

luci.bucket(name = "ci")

# This poller is still defined even though it doesn't trigger anything.
luci.gitiles_poller(
    name = "poller",
    repo = "https://noop.com",
    bucket = "ci",
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "ci"
# }
# ===
#
# === luci-scheduler.cfg
# trigger {
#   id: "poller"
#   realm: "ci"
#   acl_sets: "ci"
#   gitiles {
#     repo: "https://noop.com"
#     refs: "regexp:refs/heads/main"
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
# }
# ===
