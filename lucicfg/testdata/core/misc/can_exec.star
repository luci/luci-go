exec("//misc/support/execed.star")

lucicfg.emit(
    dest = "from-top",
    data = str(lucicfg.current_module()) + "\n",
)

# Expect configs:
#
# === from-exec
# struct(package = "@lucicfg/tests/core", path = "misc/support/execed.star")
# ===
#
# === from-top
# struct(package = "@lucicfg/tests/core", path = "misc/can_exec.star")
# ===
#
# === project.cfg
# name: "test"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# ===
