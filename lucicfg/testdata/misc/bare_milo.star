luci.project(
    name = 'infra',
    milo = 'luci-milo.appspot.com',
)

luci.milo(
    monorail_project = 'project',
    monorail_components = ['A'],
)

# Expect configs:
#
# === luci-milo.cfg
# build_bug_template: <
#   monorail_project: "project"
#   components: "A"
# >
# ===
#
# === project.cfg
# name: "infra"
# ===
