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
    combine_cls_stabilization_delay = 10 * time.second,
    allow_submit_with_open_deps = True,
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //cq/combine_cls_incompatible.star: in <toplevel>
#   ...
# Error: combine_cls_stabilization_delay and allow_submit_with_open_deps are both set.
