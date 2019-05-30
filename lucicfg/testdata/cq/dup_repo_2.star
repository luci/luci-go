luci.project(
    name = 'zzz',
    acls = [acl.entry(acl.CQ_COMMITTER, groups = ['g'])],
)

luci.cq_group(
    name = 'group 1',
    watch = cq.refset(
        repo = 'https://example.googlesource.com/repo',
        refs = ['a', 'b'],
    ),
)

# This is fine.
luci.cq_group(
    name = 'group 2',
    watch = cq.refset(
        repo = 'https://example.googlesource.com/a/repo',
        refs = ['c'],
    ),
)

# This is not.
luci.cq_group(
    name = 'group 3',
    watch = cq.refset(
        repo = 'https://example.googlesource.com/a/repo',
        refs = ['a', 'd'],
    ),
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/cq/dup_repo_2.star: in <toplevel>
#   ...
# Error: ref regexp "a" of "https://example.googlesource.com/a/repo" is already covered by a cq_group, previous declaration:
# Traceback (most recent call last):
#   //testdata/cq/dup_repo_2.star: in <toplevel>
#   ...
