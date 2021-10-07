luci.project(
    name = "foo",
    tricium = "tricium-prod.appspot.com",
)

luci.cq_group(
    name = "main",
    watch = [
        cq.refset("https://example.googlesource.com/repo1"),
        cq.refset("https://example.googlesource.com/repo2"),
    ],
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ["committer"]),
    ],
    verifiers = [
        luci.cq_tryjob_verifier(
            builder = "infra:analyzer/go-linter",
            owner_whitelist = ["project-contributor"],
            location_regexp = [
                r"https://example-review.googlesource.com/repo1/[+]/.+\.go",
            ],
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
        luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            owner_whitelist = ["project-contributor"],
            location_regexp = [
                r"https://example-review.googlesource.com/repo1/[+]/.+",
                r"https://example-review.googlesource.com/repo2/[+]/.+",
            ],
            mode_allowlist = [cq.MODE_ANALYZER_RUN],
        ),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/tricium/analyzers_watch_different_repos.star: in <toplevel>
#   ...
# Error: The location_regexp of analyzer luci.cq_tryjob_verifier("spell-checker") sepcifies different set of gerrit repos from the other anaylzer; got: ["example-review.googlesource.com/repo1", "example-review.googlesource.com/repo2"] other: ["example-review.googlesource.com/repo1"]
