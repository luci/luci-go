luci.notifier(
    name = "wrong",
    on_new_status = ["FAILURE"],
    failed_step_regexp = "some_regex",
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //errors/notify_regexes_and_on_new_status.star: in <toplevel>
#   ...
# Error: failed step regexes cannot be used in combination with status change predicates
