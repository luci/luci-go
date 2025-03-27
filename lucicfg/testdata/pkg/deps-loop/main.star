load("//lib.star", "say_hi")
say_hi("main")
exec("@lucicfg/dep//dep.star")

# Expect configs:
#
# === dep.cfg
# Hi
# ===
#
# === main.cfg
# Hi
# ===
