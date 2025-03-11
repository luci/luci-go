luci.project(
    name = "zzz",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)
luci.bucket(name = "dynamic", dynamic = True)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "dynamic"
#   dynamic_builder_template {}
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
#   name: "dynamic"
# }
# ===
