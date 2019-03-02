luci.project(
    name = 'zzz',
    acls = [acl.entry(acl.CQ_COMMITTER, groups = ['g'])],
)

luci.cq_group(
    name = 'group',
    watch = [
        cq.refset('https://example.googlesource.com/repo'),
        cq.refset('https://example.googlesource.com/a/repo.git'),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/cq/dup_repo_1.star:6: in <toplevel>
#   ...
# Error: ref regexp "refs/heads/master" of "https://example.googlesource.com/a/repo.git" is already covered by a cq_group, previous declaration:
# Traceback (most recent call last):
#   //testdata/cq/dup_repo_1.star:6: in <toplevel>
#   ...
