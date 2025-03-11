luci.project(
    name = "proj",
    bindings = [
        # Binding across different roles are independent.
        luci.binding(
            roles = "role/a",
            groups = "g1",
            conditions = [
                luci.restrict_attribute("attr1", ["val1"]),
            ],
        ),
        luci.binding(
            roles = "role/b",
            groups = "g1",
            conditions = [
                luci.restrict_attribute("attr1", ["val1"]),
            ],
        ),

        # Groups bindings with semantically identical conditions.
        luci.binding(
            roles = "role/a",
            groups = "g3",
            conditions = [
                luci.restrict_attribute("attr1", ["val1", "val2"]),
                luci.restrict_attribute("attr2", ["val1", "val2"]),
            ],
        ),
        luci.binding(
            roles = "role/a",
            groups = "g4",
            conditions = [
                luci.restrict_attribute("attr2", ["val2", "val1"]),
                luci.restrict_attribute("attr1", ["val2", "val1"]),
            ],
        ),

        # Condition-less binding must sort before conditional ones.
        luci.binding(
            roles = "role/a",
            groups = "g1",
        ),
    ],
)

# Expect configs:
#
# === project.cfg
# name: "proj"
# ===
#
# === realms.cfg
# realms {
#   name: "@root"
#   bindings {
#     role: "role/a"
#     principals: "group:g1"
#   }
#   bindings {
#     role: "role/a"
#     principals: "group:g1"
#     conditions {
#       restrict {
#         attribute: "attr1"
#         values: "val1"
#       }
#     }
#   }
#   bindings {
#     role: "role/a"
#     principals: "group:g3"
#     principals: "group:g4"
#     conditions {
#       restrict {
#         attribute: "attr1"
#         values: "val1"
#         values: "val2"
#       }
#     }
#     conditions {
#       restrict {
#         attribute: "attr2"
#         values: "val1"
#         values: "val2"
#       }
#     }
#   }
#   bindings {
#     role: "role/b"
#     principals: "group:g1"
#     conditions {
#       restrict {
#         attribute: "attr1"
#         values: "val1"
#       }
#     }
#   }
# }
# ===
