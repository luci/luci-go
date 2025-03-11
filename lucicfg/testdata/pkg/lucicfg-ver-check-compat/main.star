lucicfg.check_version("1.1.0")

# Expect errors:
#
# Traceback (most recent call last):
#   //main.star: in <toplevel>
#   @stdlib//internal/lucicfg.star: in _check_version
#   <builtin>: in fail
# Error: Version passed to lucicfg.check_version (which is "1.1.0") should match the lucicfg version in pkg.declare(...) in PACKAGE.star (which is "1.1.1"). Eventually pkg.declare(...) in PACKAGE.star will become authoritative and lucicfg.check_version will be retired. Until then the versions must agree. Please update lucicfg.check_version(...) call.
