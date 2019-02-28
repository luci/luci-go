luci.project(name = 'zzz')

luci.cq_group(
    name = 'group 1',
    watch = cq.refset('https://example.googlesource.com/repo'),
)

luci.cq_group(
    name = 'group 2',
    watch = cq.refset('https://example.googlesource.com/a/repo.git'),
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/cq/dup_repo_2.star:8: in <toplevel>
#   ...
# Error: repo "https://example.googlesource.com/a/repo.git" is already covered by another cq_group, previous declaration:
# Traceback (most recent call last):
#   //testdata/cq/dup_repo_2.star:3: in <toplevel>
#   ...
