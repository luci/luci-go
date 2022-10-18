# This example includes a case where location_filters and location_regexp are
# used in the same config. This should be disallowed because of ambiguity in
# how it would be interpreted.

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
            location_regexp_exclude = [r".+/[+]/3pp/.+"],
            location_filters = [cq.location_filter(path_regexp = "3pp/.+", exclude = True)],
        ),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/cq/location_filter_conflict.star: in <toplevel>
#   ...
# Error: "location_filters" can not be used together with "location_regexp" or "location_regexp_exclude"
