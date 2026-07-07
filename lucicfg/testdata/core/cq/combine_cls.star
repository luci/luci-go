luci.project(
    name = "foo",
)

luci.cq_group(
    name = "main",
    watch = [
        cq.refset("https://example.googlesource.com/proj/repo"),
    ],
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ["committer"]),
    ],
    combine_cls_stabilization_delay = 10 * time.second,
)

# Expect configs:
#
# === commit-queue.cfg
# config_groups {
#   name: "main"
#   gerrit {
#     url: "https://example-review.googlesource.com"
#     projects {
#       name: "proj/repo"
#       ref_regexp: "refs/heads/main"
#     }
#   }
#   combine_cls {
#     stabilization_delay {
#       seconds: 10
#     }
#   }
#   verifiers {
#     gerrit_cq_ability {
#       committer_list: "committer"
#     }
#   }
# }
# ===
#
# === project.cfg
# name: "foo"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# ===
