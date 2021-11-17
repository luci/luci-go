luci.project(
    name = "proj",
    buildbucket = "cr-buildbucket.appspot.com",
    bindings = [
        luci.binding(
            roles = "role/a",
            groups = "root",
        ),
    ],
    enforce_realms_in = ["cr-buildbucket"],
)

luci.bucket(
    name = "bucket",
    bindings = [
        luci.binding(
            roles = "role/a",
            groups = "bucket",
        ),
        luci.binding(
            roles = "role/empty-binding-is-skipped",
            groups = [],
        ),
    ],
)

luci.realm(
    name = "realm1",
    extends = "bucket",
    bindings = [
        luci.binding(
            roles = "role/a",
            groups = "group-a",
            users = "a@example.com",
            projects = "proj-a",
        ),
    ],
)

luci.bucket(
    name = "bucket2",
    extends = "realm1",
    bindings = [
        luci.binding(
            roles = "role/a",
            groups = "bucket",
        ),
    ],
)

luci.bucket(
    name = "bucket3",
    extends = luci.bucket(name = "bucket4"),
)

luci.binding(
    realm = ["realm1", "realm2"],
    roles = ["role/a", "role/b"],
    groups = ["group-a", "group-b"],
    users = ["a@example.com", "b@example.com"],
    projects = ["proj-a", "proj-b"],
)

luci.realm(
    name = "realm2",
    extends = ["bucket", "realm1", "@root"],
    bindings = [
        luci.binding(roles = "role/c", groups = "group-c"),
        luci.binding(roles = "role/c", users = "c@example.com"),
        luci.binding(roles = "role/c", projects = "proj-c"),
    ],
)

luci.realm(
    name = "@legacy",
    extends = "@root",
)

# Empty binding is fine.
luci.binding(
    realm = "@legacy",
    roles = "role/a",
)

luci.binding(
    realm = "@legacy",
    roles = "role/a",
    users = "a@example.com",
)

luci.custom_role(
    name = "customRole/r1",
    extends = [
        "role/a",
        luci.custom_role(
            name = "customRole/r2",
            extends = ["customRole/r3"],
            permissions = ["luci.dev.testing2"],
        ),
    ],
    permissions = ["luci.dev.testing1"],
)

luci.custom_role(
    name = "customRole/r3",
    permissions = ["luci.dev.testing3"],
)

luci.realm(
    name = "custom_roles",
    bindings = [
        luci.binding(
            roles = "customRole/r1",
            users = "a@example.com",
        ),
        luci.binding(
            roles = ["role/a", "customRole/r1"],
            users = "b@example.com",
        ),
        luci.binding(
            roles = luci.custom_role(
                name = "customRole/r3",
                permissions = ["luci.dev.testing3"],
            ),
            users = "c@example.com",
        ),
        luci.binding(
            roles = [
                luci.custom_role(
                    name = "customRole/r3",
                    permissions = ["luci.dev.testing3"],
                ),
                "role/a",
            ],
            users = "d@example.com",
        ),
    ],
)

# Expect configs:
#
# === cr-buildbucket.cfg
# buckets {
#   name: "bucket"
# }
# buckets {
#   name: "bucket2"
# }
# buckets {
#   name: "bucket3"
# }
# buckets {
#   name: "bucket4"
# }
# ===
#
# === project.cfg
# name: "proj"
# ===
#
# === realms.cfg
# realms {
#   name: "@legacy"
#   bindings {
#     role: "role/a"
#     principals: "user:a@example.com"
#   }
# }
# realms {
#   name: "@root"
#   bindings {
#     role: "role/a"
#     principals: "group:root"
#   }
#   enforce_in_service: "cr-buildbucket"
# }
# realms {
#   name: "bucket"
#   bindings {
#     role: "role/a"
#     principals: "group:bucket"
#   }
# }
# realms {
#   name: "bucket2"
#   extends: "realm1"
#   bindings {
#     role: "role/a"
#     principals: "group:bucket"
#   }
# }
# realms {
#   name: "bucket3"
#   extends: "bucket4"
# }
# realms {
#   name: "bucket4"
# }
# realms {
#   name: "custom_roles"
#   bindings {
#     role: "customRole/r1"
#     principals: "user:a@example.com"
#     principals: "user:b@example.com"
#   }
#   bindings {
#     role: "customRole/r3"
#     principals: "user:c@example.com"
#     principals: "user:d@example.com"
#   }
#   bindings {
#     role: "role/a"
#     principals: "user:b@example.com"
#     principals: "user:d@example.com"
#   }
# }
# realms {
#   name: "realm1"
#   extends: "bucket"
#   bindings {
#     role: "role/a"
#     principals: "group:group-a"
#     principals: "group:group-b"
#     principals: "project:proj-a"
#     principals: "project:proj-b"
#     principals: "user:a@example.com"
#     principals: "user:b@example.com"
#   }
#   bindings {
#     role: "role/b"
#     principals: "group:group-a"
#     principals: "group:group-b"
#     principals: "project:proj-a"
#     principals: "project:proj-b"
#     principals: "user:a@example.com"
#     principals: "user:b@example.com"
#   }
# }
# realms {
#   name: "realm2"
#   extends: "bucket"
#   extends: "realm1"
#   bindings {
#     role: "role/a"
#     principals: "group:group-a"
#     principals: "group:group-b"
#     principals: "project:proj-a"
#     principals: "project:proj-b"
#     principals: "user:a@example.com"
#     principals: "user:b@example.com"
#   }
#   bindings {
#     role: "role/b"
#     principals: "group:group-a"
#     principals: "group:group-b"
#     principals: "project:proj-a"
#     principals: "project:proj-b"
#     principals: "user:a@example.com"
#     principals: "user:b@example.com"
#   }
#   bindings {
#     role: "role/c"
#     principals: "group:group-c"
#     principals: "project:proj-c"
#     principals: "user:c@example.com"
#   }
# }
# custom_roles {
#   name: "customRole/r1"
#   extends: "customRole/r2"
#   extends: "role/a"
#   permissions: "luci.dev.testing1"
# }
# custom_roles {
#   name: "customRole/r2"
#   extends: "customRole/r3"
#   permissions: "luci.dev.testing2"
# }
# custom_roles {
#   name: "customRole/r3"
#   permissions: "luci.dev.testing3"
# }
# ===
