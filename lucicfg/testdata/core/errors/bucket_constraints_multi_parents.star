luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
)

luci.bucket(name = "ci")
luci.bucket(
    name = "ci.shadow",
    shadows = "ci",
    constraints = luci.bucket_constraints(
        bucket = "ci",
        pools = ["luci.project.shadow"],
        service_accounts = ["shadow-sa@chops-service-account.com"],
    ),
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/bucket_constraints_multi_parents.star: in <toplevel>
#   ...
# Error: luci.bucket_constraints("ci") is added to multiple buckets: luci.bucket("ci"), luci.bucket("ci.shadow")
# ...
