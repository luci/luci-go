core.project(
    name = 'proj',
    # no 'buildbucket' attribute
)

core.bucket(name = 'ci')

# TODO(vadimsh): Filter out stdlib@ frames from public stack traces.

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/bucket_without_service.star:1: in <toplevel>
#   ...
# Error: missing "buildbucket" in core.project(...), it is required for defining buckets
