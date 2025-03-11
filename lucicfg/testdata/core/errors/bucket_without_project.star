luci.bucket(name = "ci")

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/bucket_without_project.star: in <toplevel>
#   ...
# Error: luci.bucket("ci") refers to undefined luci.project("...")
# ...
