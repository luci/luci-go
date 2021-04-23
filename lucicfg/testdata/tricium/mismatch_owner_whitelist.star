luci.project(
    name = "foo",
    tricium = "tricium-prod.appspot.com",
)

luci.cq_group(
    name = "main",
    watch = cq.refset("https://example.googlesource.com/repo1"),
    acls = [
        acl.entry(acl.CQ_COMMITTER, groups = ["committer"]),
    ],
    verifiers = [
        luci.cq_tryjob_verifier(
            builder = "infra:analyzer/spell-checker",
            owner_whitelist = ["project-contributor"],
            mode_regexp = [cq.MODE_ANALYZER_RUN],
        ),
        luci.cq_tryjob_verifier(
            builder = "infra:analyzer/format-checker",
            owner_whitelist = ["all"],
            mode_regexp = [cq.MODE_ANALYZER_RUN],
        ),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/tricium/mismatch_owner_whitelist.star: in <toplevel>
#   ...
# Error: analyzer luci.cq_tryjob_verifier("format-checker") has different owner_whitelist from other analyzers in the config group luci.cq_group("main")
