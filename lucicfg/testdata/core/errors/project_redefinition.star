luci.project(name = "proj")
luci.project(name = "another")

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/project_redefinition.star: in <toplevel>
#   ...
# Error in add_node: luci.project("...") is redeclared, previous declaration:
# Traceback (most recent call last):
#   //errors/project_redefinition.star: in <toplevel>
#   ...
