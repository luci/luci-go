luci.project(
    name = "foo",
    tricium = "tricium-prod.appspot.com",
)

luci.cq_group(
    name = "watching repo1",
    watch = cq.refset("https://example.googlesource.com/repo1"),
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ["committer"]),
    ],
    verifiers = [
        luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            mode_regexp = [cq.MODE_ANALYZER_RUN],
        ),
    ],
)

luci.cq_group(
    name = "watching repo2",
    watch = cq.refset("https://example.googlesource.com/repo2"),
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ["committer"]),
    ],
    verifiers = [
        luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            mode_regexp = [cq.MODE_ANALYZER_RUN],
        ),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/tricium/invalid_multi_cq_groups.star: in <toplevel>
#   ...
# Error: luci.cq_group("watching repo2") is watching different set of Gerrit repos or defining different analyzers from luci.cq_group("watching repo1")
