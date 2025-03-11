luci.project(name = "proj")
luci.realm(name = "some", extends = "undefined")

# Expect errors like:
#
# Traceback (most recent call last):
#   //realms/bad_ref.star: in <toplevel>
#   ...
# Error: luci.realm("some") in "extends" refers to undefined luci.realm("undefined")
