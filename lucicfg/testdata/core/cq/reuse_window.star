luci.project(
    name = "chromium",
)

luci.cq_group(
    name = "main",
    watch = [
        cq.refset("https://chromium.googlesource.com/chromium/src"),
    ],
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ["committer"]),
    ],
    verifiers = [
        # Standard commit-distance reuse window
        luci.cq_tryjob_verifier(
            builder = "chromium:try/linux-rel",
            reuse_max_commit_distance = 1000,
        ),
        # Strict commit-distance reuse window with footer invalidation
        luci.cq_tryjob_verifier(
            builder = "chromium:try/win-rel",
            disable_reuse_footers = ["Include-Ci-Only-Tests"],
            reuse_max_commit_distance = 300,
        ),
        # Presubmit verifier (reuse unconditionally disabled)
        luci.cq_tryjob_verifier(
            builder = "chromium:try/presubmit",
            disable_reuse = True,
        ),
        # Default verifier (omits commit distance, defaults to legacy 24 h window)
        luci.cq_tryjob_verifier(
            builder = "chromium:try/mac-rel",
        ),
    ],
)

# Expect configs:
#
# === commit-queue.cfg
# config_groups {
#   name: "main"
#   gerrit {
#     url: "https://chromium-review.googlesource.com"
#     projects {
#       name: "chromium/src"
#       ref_regexp: "refs/heads/main"
#     }
#   }
#   verifiers {
#     gerrit_cq_ability {
#       committer_list: "committer"
#     }
#     tryjob {
#       builders {
#         name: "chromium/try/linux-rel"
#         reuse_window {
#           max_commit_distance: 1000
#         }
#       }
#       builders {
#         name: "chromium/try/mac-rel"
#       }
#       builders {
#         name: "chromium/try/presubmit"
#         disable_reuse: true
#       }
#       builders {
#         name: "chromium/try/win-rel"
#         disable_reuse_footers: "Include-Ci-Only-Tests"
#         reuse_window {
#           max_commit_distance: 300
#         }
#       }
#       retry_config {
#         single_quota: 1
#         global_quota: 2
#         failure_weight: 100
#         transient_failure_weight: 1
#         timeout_weight: 100
#       }
#     }
#   }
# }
# ===
#
# === project.cfg
# name: "chromium"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# ===
