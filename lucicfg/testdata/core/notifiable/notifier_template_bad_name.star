luci.notifier_template(
    name = "template-with-dashes-in-name",
    body = "foo",
)

# Expect errors:
#
# Traceback (most recent call last):
#   //notifiable/notifier_template_bad_name.star: in <toplevel>
#   <builtin>: in _notifier_template
#   @stdlib//internal/validate.star: in _string
#   <builtin>: in fail
# Error: bad "name": "template-with-dashes-in-name" should match "^[a-z][a-z0-9\\_]*$"
