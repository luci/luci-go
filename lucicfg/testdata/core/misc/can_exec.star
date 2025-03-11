exec("//misc/support/execed.star")

lucicfg.emit(
    dest = "from-top",
    data = str(lucicfg.current_module()) + "\n",
)

# Expect configs:
#
# === from-exec
# struct(package = "__main__", path = "misc/support/execed.star")
# ===
#
# === from-top
# struct(package = "__main__", path = "misc/can_exec.star")
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
