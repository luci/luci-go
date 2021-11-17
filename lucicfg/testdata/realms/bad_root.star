luci.project(name = "proj")
luci.realm(name = "@root", extends = "something")

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/realms/bad_root.star: in <toplevel>
#   ...
# Error: @root realm can't extend other realms
