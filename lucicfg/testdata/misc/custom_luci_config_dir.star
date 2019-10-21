luci.project(
    name = 'project',
    config_dir = 'abc/./././/def',
)

# Expect configs:
#
# === abc/def/project.cfg
# name: "project"
# ===
