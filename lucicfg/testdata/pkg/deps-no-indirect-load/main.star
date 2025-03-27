load("@lucicfg/dep1//dep.star", "sym1") # works
load("@lucicfg/dep2//dep.star", "sym2") # fails

# Expect errors:
#
#
# Traceback (most recent call last):
#   //main.star: in <toplevel>
# Error: cannot load @lucicfg/dep2//dep.star: @lucicfg/tests doesn't directly depend on @lucicfg/dep2 in its PACKAGE.star
