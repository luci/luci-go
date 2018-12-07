core.bucket(name = 'ci')

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/bucket_without_project.star:1: in <toplevel>
#   ...
# Error: core.bucket("ci") refers to undefined core.project("...")
