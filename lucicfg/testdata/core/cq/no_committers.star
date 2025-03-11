luci.project(name = "zzz")

luci.cq_group(
    name = "group",
    watch = cq.refset("https://example.googlesource.com/repo"),
    acls = [
        acl.entry(acl.CQ_DRY_RUNNER, groups = ["running dry"]),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //cq/no_committers.star: in <toplevel>
#   ...
# Error: at least one CQ_COMMITTER acl.entry must be specified (either here or in luci.project)
