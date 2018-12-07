core.project(name = "proj")
core.project(name = "another")

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/project_redefinition.star:2: in <toplevel>
#   ...
# Error: core.project("...") is redeclared, previous declaration:
# Traceback (most recent call last):
#   //testdata/errors/project_redefinition.star:1: in <toplevel>
#   ...
