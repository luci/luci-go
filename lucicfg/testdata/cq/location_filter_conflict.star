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
