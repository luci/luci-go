lucicfg.config(lint_checks = ["default", "+formatting"])

# Expect errors:
#
# Traceback (most recent call last):
#   //main.star: in <toplevel>
#   @stdlib//internal/lucicfg.star: in _config
#   <builtin>: in fail
# Error: Lint checks set via lucicfg.config(...) (which are ["default", "+formatting"]) do not match lint checks set via pkg.options.lint_checks(...) in PACKAGE.star (which are ["default"]). Eventually pkg.options.lint_checks(...) in PACKAGE.star will become authoritative and setting lint checks via lucicfg.config(...) will be retired. Until then the values must agree. Please update lucicfg.config(...) call.
