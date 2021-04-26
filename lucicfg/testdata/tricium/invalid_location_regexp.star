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
            location_regexp = [r".+\.md", ".+/docs/.+"],
            mode_regexp = [cq.MODE_ANALYZER_RUN],
        ),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/tricium/invalid_location_regexp.star: in <toplevel>
#   ...
# Error: "location_regexp" of an analyzer MUST start with ".+\." and followed by file extension
