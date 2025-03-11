luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
)
luci.bucket(name = "shadow", shadows = ["unknown-bucket"])

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/shadow_bucket_no_shadowed_bucket.star: in <toplevel>
#   ...
# Error: luci.bucket("shadow") in "shadows" refers to undefined luci.shadowed_bucket("unknown-bucket")
# ...
