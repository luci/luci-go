luci.bucket(name = 'ci')

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/bucket_without_project.star:1: in <toplevel>
#   ...
# Error: luci.bucket("ci") refers to undefined luci.project("...")
