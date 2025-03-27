exec("@lucicfg/overridden-1//exec.star")
exec("@lucicfg/remote-1//exec.star")
exec("@lucicfg/remote-2//exec.star")

# Expect configs:
#
# === overridden-repo-1.cfg
# ...
# ===
#
# === overridden-repo-2-local.cfg
# ...
# ===
#
# === overridden-repo-2.cfg
# ...
# ===
#
# === remote-repo-1-v2.cfg
# ...
# ===
