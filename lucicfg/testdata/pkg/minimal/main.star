lucicfg.check_version("1.1.1")
lucicfg.config(lint_checks = ["default", "+formatting"])

luci.project(name = "minimal")

# Expect configs:
#
# === project.cfg
# name: "minimal"
# lucicfg {
#   version: "1.1.1"
#   package_name: "@lucicfg/tests"
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
