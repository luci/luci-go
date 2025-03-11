luci.project(name = "proj")
luci.realm(
    name = "realm",
    bindings = [
        luci.binding(
            roles = "customRole/undefined",
            users = "a@example.com",
        ),
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //realms/bad_binding_role.star: in <toplevel>
#   ...
# Error: luci.binding("customRole/undefined") in "roles" refers to undefined luci.custom_role("customRole/undefined")
