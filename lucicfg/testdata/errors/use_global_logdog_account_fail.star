luci.project(
    name = "project",
    logdog = "luci-logdog.appspot.com",
)

luci.logdog(use_global_logdog_account=True)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/use_global_logdog_account_fail.star: in <toplevel>
#   ...
# Error: use_global_logdog_account: must be False, if cloud_logging_project is unset
