lucicfg.enable_experiment('crbug.com/1085650')

luci.project(name = 'proj')
luci.realm(name = '@root')

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/realms/redef.star: in <toplevel>
#   ...
# Error: luci.realm("@root") is redeclared, previous declaration:
# Traceback (most recent call last):
#   //testdata/realms/redef.star: in <toplevel>
#   ...
