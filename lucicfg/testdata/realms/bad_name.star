luci.project(name = "proj")
luci.realm(name = "not allowed")

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/realms/bad_name.star: in <toplevel>
#   ...
# Error: bad "name": "not allowed" should match "^([a-z0-9_\\.\\-/]{1,400}|@root|@legacy|@project)$"
