luci.project(name = "proj")
luci.custom_role(
    name = "customRole/r1",
    extends = ["customRole/r2"],
)
luci.custom_role(
    name = "customRole/r2",
    extends = ["customRole/r1"],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //realms/role_cycle.star: in <toplevel>
#   ...
# Error in add_edge: relation "extends" between luci.custom_role("customRole/r1") and luci.custom_role("customRole/r2") introduces a cycle
