luci.logdog()

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/logdog_without_project.star:1: in <toplevel>
#   ...
# Error: luci.logdog("...") refers to undefined luci.project("...")
