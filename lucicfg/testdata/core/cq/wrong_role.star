luci.project(name = "zzz")

luci.cq_group(
    name = "group",
    watch = cq.refset("https://example.googlesource.com/repo"),
    acls = [
        acl.entry(acl.BUILDBUCKET_READER, users = ["a@example.com"]),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //cq/wrong_role.star: in <toplevel>
#   ...
# Error: bad "acls": role BUILDBUCKET_READER is not allowed in this context
