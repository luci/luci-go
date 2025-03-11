luci.bucket_constraints(
    bucket = "ci.shadow",
    pools = ["luci.project.shadow"],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/bucket_constraints_bucket_undefined.star: in <toplevel>
#   ...
# Error: luci.bucket_constraints("ci.shadow") refers to undefined luci.bucket("ci.shadow")
# ...
