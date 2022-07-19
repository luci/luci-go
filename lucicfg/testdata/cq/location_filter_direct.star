# This example includes examples of location_regexp[_exclude] filters that are
# used in real configs, in order to test generation of location_filters.
# TODO(crbug/1171945): This test could be removed after location_regexp is
# removed.

lucicfg.enable_experiment("crbug.com/1171945")

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
    verifiers = [
        luci.cq_tryjob_verifier(
            builder = "infra:analyzer/go-linter",
            location_filters = [
                cq.location_filter(path_regexp = "3pp/.+"),
                cq.location_filter(path_regexp = "dashboard/.+"),
                cq.location_filter(gerrit_project_regexp = ".*manifest"),
                cq.location_filter(gerrit_project_regexp = ".+", path_regexp = ".*/OWNERS"),
                cq.location_filter(path_regexp = "3pp/exception/.+", exclude = True),
                cq.location_filter(
                    gerrit_host_regexp = "example.com",
                    gerrit_project_regexp = "repo",
                    path_regexp = "all/one.txt",
                    exclude = True,
                ),
                cq.location_filter(
                    gerrit_host_regexp = "example.com",
                    gerrit_project_regexp = "external/github.com/repo",
                    path_regexp = "all/one.txt",
                    exclude = True,
                ),
            ],
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
#       name: "proj/repo"
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
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".*"
#           path_regexp: "3pp/.+"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".*"
#           path_regexp: "dashboard/.+"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".*manifest"
#           path_regexp: ".*"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".+"
#           path_regexp: ".*/OWNERS"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".*"
#           path_regexp: "3pp/exception/.+"
#           exclude: true
#         }
#         location_filters {
#           gerrit_host_regexp: "example.com"
#           gerrit_project_regexp: "repo"
#           path_regexp: "all/one.txt"
#           exclude: true
#         }
#         location_filters {
#           gerrit_host_regexp: "example.com"
#           gerrit_project_regexp: "external/github.com/repo"
#           path_regexp: "all/one.txt"
#           exclude: true
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
# name: "foo"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# ===
