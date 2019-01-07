load("another.star", "sym1", sym2="exported")

exported_func = sym2.func

stuff = struct(
  a = struct(
    b = exported_func,
  ),
)

zzz = stuff.a.b
