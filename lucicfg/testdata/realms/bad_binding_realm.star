lucicfg.enable_experiment('crbug.com/1085650')

luci.project(name = 'proj')
luci.binding(
    realm = 'unknown',
    roles = 'role/a',
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/realms/bad_binding_realm.star: in <toplevel>
#   ...
# Error: luci.binding("role/a") refers to undefined luci.realm("unknown")
