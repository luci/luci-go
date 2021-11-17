luci.project(name = "zzz")

luci.cq_group(
    name = "first",
    watch = cq.refset("https://example.googlesource.com/repo"),
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ["committers"]),
    ],
)

# Expect configs:
#
# === commit-queue.cfg
# config_groups {
#   name: "first"
#   gerrit {
#     url: "https://example-review.googlesource.com"
#     projects {
#       name: "repo"
#       ref_regexp: "refs/heads/main"
#     }
#   }
#   verifiers {
#     gerrit_cq_ability {
#       committer_list: "committers"
#     }
#   }
# }
# ===
#
# === project.cfg
# name: "zzz"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# ===
