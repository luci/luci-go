luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
)
luci.bucket(name = "bucket")

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "bucket"
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
#   name: "bucket"
# }
# ===
