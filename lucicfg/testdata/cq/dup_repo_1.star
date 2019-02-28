luci.project(name = 'zzz')

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
#   //testdata/cq/dup_repo_1.star:3: in <toplevel>
#   ...
# Error: bad "watch": repository "https://example.googlesource.com/a/repo.git" is mentioned more than once, please merge the definitions
