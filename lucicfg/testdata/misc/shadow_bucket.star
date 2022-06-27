luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
)

# A bucket shadows another bucket.
luci.bucket(name = "bucket")
luci.bucket(name = "bucket.shadow", shadows = "bucket")

# A bucket shadow itself.
luci.bucket(name = "self_shadow", shadows = "self_shadow")

# A bucket shadows multiple buckets.
luci.bucket(name = "b1")
luci.bucket(name = "b2")
luci.bucket(name = "adhoc", shadows = ["b1", "b2"])

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "adhoc"
# }
# buckets {
#   name: "b1"
#   shadow: "adhoc"
# }
# buckets {
#   name: "b2"
#   shadow: "adhoc"
# }
# buckets {
#   name: "bucket"
#   shadow: "bucket.shadow"
# }
# buckets {
#   name: "bucket.shadow"
# }
# buckets {
#   name: "self_shadow"
#   shadow: "self_shadow"
# }
# ===
#
# === project.cfg
# name: "zzz"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# realms {
#   name: "adhoc"
# }
# realms {
#   name: "b1"
# }
# realms {
#   name: "b2"
# }
# realms {
#   name: "bucket"
# }
# realms {
#   name: "bucket.shadow"
# }
# realms {
#   name: "self_shadow"
# }
# ===
