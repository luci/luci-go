luci.project(
    name = "foo",
    tricium = "tricium-prod.appspot.com",
)

luci.cq_group(
    name = "main",
    watch = [
        cq.refset("https://example.googlesource.com/proj/repo1"),
        cq.refset("https://example-internal.googlesource.com/proj/repo2"),
        cq.refset("https://example.googlesource.com/proj/repo3"),
        cq.refset("https://example-internal.googlesource.com/proj/repo4"),
    ],
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ["committer"]),
    ],
    verifiers = [
        luci.cq_tryjob_verifier(
            builder = "infra:analyzer/go-linter",
            owner_whitelist = ["project-contributor"],
            location_filters = [
                cq.location_filter(
                    gerrit_host_regexp = "example-review.googlesource.com",
                    gerrit_project_regexp = "proj/repo1",
                    path_regexp = ".+\\.go",
                ),
                cq.location_filter(
                    gerrit_host_regexp = "example-internal-review.googlesource.com",
                    gerrit_project_regexp = "proj/repo2",
                    path_regexp = ".+\\.go",
                ),
            ],
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
        luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            owner_whitelist = ["project-contributor"],
            location_filters = [
                cq.location_filter(
                    gerrit_host_regexp = "example-review.googlesource.com",
                    gerrit_project_regexp = "proj/repo1",
                    path_regexp = ".*",
                ),
                cq.location_filter(
                    gerrit_host_regexp = "example-internal-review.googlesource.com",
                    gerrit_project_regexp = "proj/repo2",
                    path_regexp = ".+",
                ),
            ],
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
#       name: "proj/repo1"
#       ref_regexp: "refs/heads/main"
#     }
#     projects {
#       name: "proj/repo3"
#       ref_regexp: "refs/heads/main"
#     }
#   }
#   gerrit {
#     url: "https://example-internal-review.googlesource.com"
#     projects {
#       name: "proj/repo2"
#       ref_regexp: "refs/heads/main"
#     }
#     projects {
#       name: "proj/repo4"
#       ref_regexp: "refs/heads/main"
#     }
#   }
#   verifiers {
#     gerrit_cq_ability {
#       committer_list: "committer"
#     }
#     tryjob {
#       builders {
#         name: "infra/analyzer/go-linter"
#         location_filters {
#           gerrit_host_regexp: "example-review.googlesource.com"
#           gerrit_project_regexp: "proj/repo1"
#           gerrit_ref_regexp: ".*"
#           path_regexp: ".+\\.go"
#         }
#         location_filters {
#           gerrit_host_regexp: "example-internal-review.googlesource.com"
#           gerrit_project_regexp: "proj/repo2"
#           gerrit_ref_regexp: ".*"
#           path_regexp: ".+\\.go"
#         }
#         owner_whitelist_group: "project-contributor"
#         mode_allowlist: "ANALYZER_RUN"
#       }
#       builders {
#         name: "infra/analyzer/spell-checker"
#         location_filters {
#           gerrit_host_regexp: "example-review.googlesource.com"
#           gerrit_project_regexp: "proj/repo1"
#           gerrit_ref_regexp: ".*"
#           path_regexp: ".*"
#         }
#         location_filters {
#           gerrit_host_regexp: "example-internal-review.googlesource.com"
#           gerrit_project_regexp: "proj/repo2"
#           gerrit_ref_regexp: ".*"
#           path_regexp: ".+"
#         }
#         owner_whitelist_group: "project-contributor"
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
#   name: "InfraAnalyzerGoLinter"
#   needs: GIT_FILE_DETAILS
#   provides: RESULTS
#   path_filters: "*.go"
#   impls {
#     provides_for_platform: LINUX
#     runtime_platform: LINUX
#     recipe {
#       project: "infra"
#       bucket: "analyzer"
#       builder: "go-linter"
#     }
#   }
# }
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
#   function: "InfraAnalyzerGoLinter"
#   platform: LINUX
# }
# selections {
#   function: "InfraAnalyzerSpellChecker"
#   platform: LINUX
# }
# repos {
#   gerrit_project {
#     host: "example-review.googlesource.com"
#     project: "proj/repo1"
#     git_url: "https://example.googlesource.com/proj/repo1"
#   }
#   whitelisted_group: "project-contributor"
# }
# repos {
#   gerrit_project {
#     host: "example-internal-review.googlesource.com"
#     project: "proj/repo2"
#     git_url: "https://example-internal.googlesource.com/proj/repo2"
#   }
#   whitelisted_group: "project-contributor"
# }
# service_account: "tricium-prod@appspot.gserviceaccount.com"
# ===
