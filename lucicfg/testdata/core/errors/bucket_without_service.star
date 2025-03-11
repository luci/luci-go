luci.project(
    name = "proj",
    # no 'buildbucket' attribute
)

luci.bucket(name = "ci")

# TODO(vadimsh): Filter out stdlib@ frames from public stack traces.

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/bucket_without_service.star: in <toplevel>
#   ...
# Error: missing "buildbucket" in luci.project(...), it is required for defining buckets
