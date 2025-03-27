load("@lucicfg/dep//load.star", "say_hi")
say_hi()
exec("@lucicfg/dep//exec.star")

# Expect configs:
#
# === exec.cfg
# Hi
# ===
#
# === load.cfg
# Hi
# ===
