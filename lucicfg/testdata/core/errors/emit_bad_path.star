lucicfg.emit(
    dest = "../a.cfg",
    data = "zzz",
)

# Expect errors:
#
# Traceback (most recent call last):
#   //errors/emit_bad_path.star: in <toplevel>
# Error: path "../a.cfg" is not allowed, must not start with "../"
