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
            builder = "foo:try/linux-rel",
            reuse_max_age = 25 * time.hour,
        ),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //cq/reuse_max_age_too_long.star: in <toplevel>
#   ...
# Error: bad "reuse_max_age": 25h0m0s should be <= 24h0m0s
