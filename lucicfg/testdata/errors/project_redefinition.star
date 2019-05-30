luci.project(name = "proj")
luci.project(name = "another")

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/project_redefinition.star: in <toplevel>
#   ...
# Error: luci.project("...") is redeclared, previous declaration:
# Traceback (most recent call last):
#   //testdata/errors/project_redefinition.star: in <toplevel>
#   ...
