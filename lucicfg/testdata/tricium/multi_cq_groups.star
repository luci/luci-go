luci.project(
    name = "foo",
    tricium = "tricium-prod.appspot.com",
)

luci.cq_group(
    name = "main",
    watch = cq.refset(
        repo = "https://example.googlesource.com/repo1",
        refs = ["refs/heads/main"],
    ),
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ["committer"]),
    ],
    verifiers = [
        luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
    ],
)

luci.cq_group(
    name = "other",
    watch = cq.refset(
        repo = "https://example.googlesource.com/repo1",
        refs = [".+"],
        refs_exclude = ["refs/heads/main"],
    ),
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ["committer"]),
    ],
    verifiers = [
        luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
    ],
)

# Expect configs:
#
# === commit-queue.cfg
# config_groups {
#   name: "main"
#   gerrit {
#     url: "https://example-review.googlesource.com"
#     projects {
#       name: "repo1"
#       ref_regexp: "refs/heads/main"
#     }
#   }
#   verifiers {
#     gerrit_cq_ability {
#       committer_list: "committer"
#     }
#     tryjob {
#       builders {
#         name: "infra/analyzer/spell-checker"
#         mode_allowlist: "ANALYZER_RUN"
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
# config_groups {
#   name: "other"
#   gerrit {
#     url: "https://example-review.googlesource.com"
#     projects {
#       name: "repo1"
#       ref_regexp: ".+"
#       ref_regexp_exclude: "refs/heads/main"
#     }
#   }
#   verifiers {
#     gerrit_cq_ability {
#       committer_list: "committer"
#     }
#     tryjob {
#       builders {
#         name: "infra/analyzer/spell-checker"
#         mode_allowlist: "ANALYZER_RUN"
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
# name: "foo"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# ===
#
# === tricium-prod.cfg
# functions {
#   type: ANALYZER
#   name: "InfraAnalyzerSpellChecker"
#   needs: GIT_FILE_DETAILS
#   provides: RESULTS
#   impls {
#     provides_for_platform: LINUX
#     runtime_platform: LINUX
#     recipe {
#       project: "infra"
#       bucket: "analyzer"
#       builder: "spell-checker"
#     }
#   }
# }
# selections {
#   function: "InfraAnalyzerSpellChecker"
#   platform: LINUX
# }
# repos {
#   gerrit_project {
#     host: "example-review.googlesource.com"
#     project: "repo1"
#     git_url: "https://example.googlesource.com/repo1"
#   }
# }
# service_account: "tricium-prod@appspot.gserviceaccount.com"
# ===
