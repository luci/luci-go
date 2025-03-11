luci.project(
    name = "infra",
    milo = "luci-milo.appspot.com",
)

luci.milo(
    bug_url_template = "https://b.corp.google.com/createIssue?notes={{{milo_build_url}}}&title=build%%20{{{build.builder.build}}}%%20failed",
)

# Expect configs:
#
# === luci-milo.cfg
# bug_url_template: "https://b.corp.google.com/createIssue?notes={{{milo_build_url}}}&title=build%%20{{{build.builder.build}}}%%20failed"
# ===
#
# === project.cfg
# name: "infra"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
# }
# ===
