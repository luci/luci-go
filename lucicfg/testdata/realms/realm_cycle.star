lucicfg.enable_experiment('crbug.com/1085650')

luci.project(name = 'proj')
luci.realm(name = 'a', extends = 'b')
luci.realm(name = 'b', extends = 'a')

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/realms/realm_cycle.star: in <toplevel>
#   ...
# Error: relation "extends" between luci.realm("a") and luci.realm("b") introduces a cycle
