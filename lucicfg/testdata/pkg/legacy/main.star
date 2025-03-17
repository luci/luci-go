lucicfg.check_version("1.1.1")
lucicfg.config(lint_checks = ["default", "+formatting"])

luci.project(name = "legacy")

# Verify there's no limit on what resources can be loaded.
_ = io.read_file("resource.txt")

# Expect configs:
#
# === project.cfg
# name: "legacy"
# lucicfg {
#   version: "1.1.1"
#   package_dir: "."
#   config_dir: "."
#   entry_point: "main.star"
# }
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# ===
