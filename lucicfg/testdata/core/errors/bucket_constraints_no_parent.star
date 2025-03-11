luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
)

luci.bucket(name = "ci")

luci.bucket_constraints(
    service_accounts = ["ci-sa@chops-service-accounts.iam.gserviceaccount.com"],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/bucket_constraints_no_parent.star: in <toplevel>
#   ...
# Error: luci.bucket_constraints("") is not added to any bucket, either remove or comment it out
# ...
