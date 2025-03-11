luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
)
luci.bucket(name = "bucket")
luci.bucket(name = "shadow1", shadows = "bucket")
luci.bucket(name = "shadow2", shadows = "bucket")

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/shadow_bucket_multiple.star: in <toplevel>
#   ...
# Error in add_node: luci.shadow_of("bucket") is redeclared, previous declaration:
# ...
