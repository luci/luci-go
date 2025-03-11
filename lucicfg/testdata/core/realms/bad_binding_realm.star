luci.project(name = "proj")
luci.binding(
    realm = "unknown",
    roles = "role/a",
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //realms/bad_binding_realm.star: in <toplevel>
#   ...
# Error: luci.binding("role/a") refers to undefined luci.realm("unknown")
