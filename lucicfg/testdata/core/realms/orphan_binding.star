luci.project(name = "proj")
luci.binding(roles = "role/a")

# Expect errors like:
#
# Traceback (most recent call last):
#   //realms/orphan_binding.star: in <toplevel>
#   ...
# Error: the binding luci.binding("role/a") is not added to any realm
