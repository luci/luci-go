lucicfg.emit(
    dest = "a.cfg",
    data = "zzz",
)

lucicfg.emit(
    dest = "a.cfg",
    data = "zzz",
)

# Expect errors:
#
# Traceback (most recent call last):
#   //errors/double_emit.star: in <toplevel>
# Error: config file "a.cfg" is already generated by something else
