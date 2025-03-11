luci.project(name = "proj")
luci.custom_role(
    name = "customRole/r1",
    extends = ["customRole/r2"],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //realms/bad_parent_role.star: in <toplevel>
#   ...
# Error: luci.custom_role("customRole/r1") in "extends" refers to undefined luci.custom_role("customRole/r2")
