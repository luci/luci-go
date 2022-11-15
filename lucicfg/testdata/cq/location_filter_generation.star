# This example includes examples of location_regexp[_exclude] filters that are
# used in real configs, in order to test generation of location_filters.
# TODO(crbug/1171945): This test could be removed after location_regexp is
# removed.

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
            location_regexp = [
                r".+/[+]/3pp/.+",
                r".+[+]/dashboard/.+",
                r".*manifest/[+].*",
                r".+/[+].*/OWNERS",
                r".+/touch_to_fail_tryjob",
                r".*/OWNERS",
            ],
            location_regexp_exclude = [
                r".+/[+]/3pp/exception/.+",
                r"https://example.com/repo/[+]/all/one.txt",
                r"https://example.com/external/github.com/repo/[+]/all/one.txt",
            ],
        ),
        luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            location_regexp = [
                r".+/[+]/DEPS",
                r".+[+]/.+/3pp/.+",
                r".*manifest/[+].+",
                r".+generated.+",
                r".+pubkeys.+pub",
                r".+/repo/foo/[+]/a/b/.*",
                r".+/chromeos/overlays/baseboard-dedede-private/[+]/sys-boot/coreboot-private-files-baseboard-dedede/files/blobs/.+",
                r".+/experiences/[+]/.+",
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
#         location_regexp: ".+/[+]/3pp/.+"
#         location_regexp: ".+[+]/dashboard/.+"
#         location_regexp: ".*manifest/[+].*"
#         location_regexp: ".+/[+].*/OWNERS"
#         location_regexp: ".+/touch_to_fail_tryjob"
#         location_regexp: ".*/OWNERS"
#         location_regexp_exclude: ".+/[+]/3pp/exception/.+"
#         location_regexp_exclude: "https://example.com/repo/[+]/all/one.txt"
#         location_regexp_exclude: "https://example.com/external/github.com/repo/[+]/all/one.txt"
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
#           path_regexp: "(.+/)?touch_to_fail_tryjob"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".*"
#           path_regexp: "(.*/)?OWNERS"
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
#       builders {
#         name: "infra/analyzer/spell-checker"
#         location_regexp: ".+/[+]/DEPS"
#         location_regexp: ".+[+]/.+/3pp/.+"
#         location_regexp: ".*manifest/[+].+"
#         location_regexp: ".+generated.+"
#         location_regexp: ".+pubkeys.+pub"
#         location_regexp: ".+/repo/foo/[+]/a/b/.*"
#         location_regexp: ".+/chromeos/overlays/baseboard-dedede-private/[+]/sys-boot/coreboot-private-files-baseboard-dedede/files/blobs/.+"
#         location_regexp: ".+/experiences/[+]/.+"
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".*"
#           path_regexp: "DEPS"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".*"
#           path_regexp: ".+/3pp/.+"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".*manifest"
#           path_regexp: ".+"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".*"
#           path_regexp: ".+generated.+"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: ".*"
#           path_regexp: ".+pubkeys.+pub"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: "(.+/)?repo/foo"
#           path_regexp: "a/b/.*"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: "(.+/)?chromeos/overlays/baseboard-dedede-private"
#           path_regexp: "sys-boot/coreboot-private-files-baseboard-dedede/files/blobs/.+"
#         }
#         location_filters {
#           gerrit_host_regexp: ".*"
#           gerrit_project_regexp: "(.+/)?experiences"
#           path_regexp: ".+"
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
