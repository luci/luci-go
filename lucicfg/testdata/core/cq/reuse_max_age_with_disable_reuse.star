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
            builder = "foo:try/presubmit",
            disable_reuse = True,
            reuse_max_age = 4 * time.hour,
        ),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //cq/reuse_max_age_with_disable_reuse.star: in <toplevel>
#   ...
# Error: "disable_reuse" and "reuse_max_age" cannot be used together
