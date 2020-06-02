lucicfg.enable_experiment('crbug.com/1085650')

luci.project(
    name = 'proj',

    buildbucket = 'cr-buildbucket.appspot.com',
    swarming = 'chromium-swarm.appspot.com',

    bindings = [
        luci.binding(
            roles = 'role/a',
            groups = 'root',
        ),
    ],
)

luci.bucket(
    name = 'bucket',
    bindings = [
        luci.binding(
            roles = 'role/a',
            groups = 'bucket',
        ),
    ],
)

luci.realm(
    name = 'realm1',
    extends = 'bucket',
    bindings = [
        luci.binding(
            roles = 'role/a',
            groups = 'group-a',
            users = 'a@example.com',
            projects = 'proj-a',
        ),
    ],
)

luci.binding(
    realm = ['realm1', 'realm2'],
    roles = ['role/a', 'role/b'],
    groups = ['group-a', 'group-b'],
    users = ['a@example.com', 'b@example.com'],
    projects = ['proj-a', 'proj-b'],
)

luci.realm(
    name = 'realm2',
    extends = ['bucket', 'realm1', '@root'],
    bindings = [
        luci.binding(roles = 'role/c', groups = 'group-c'),
        luci.binding(roles = 'role/c', users = 'c@example.com'),
        luci.binding(roles = 'role/c', projects = 'proj-c'),
    ],
)

luci.realm(
    name = '@legacy',
    extends = '@root',
)

# Empty binding is fine.
luci.binding(
    realm = '@legacy',
    roles = 'role/a',
)

luci.binding(
    realm = '@legacy',
    roles = 'role/a',
    users = 'a@example.com',
)


# Expect configs:
#
# === cr-buildbucket.cfg
# buckets: <
#   name: "bucket"
#   swarming: <>
# >
# ===
#
# === project.cfg
# name: "proj"
# ===
#
# === realms.cfg
# realms: <
#   name: "@legacy"
#   bindings: <
#     role: "role/a"
#     principals: "user:a@example.com"
#   >
# >
# realms: <
#   name: "@root"
#   bindings: <
#     role: "role/a"
#     principals: "group:root"
#   >
# >
# realms: <
#   name: "bucket"
#   bindings: <
#     role: "role/a"
#     principals: "group:bucket"
#   >
# >
# realms: <
#   name: "realm1"
#   extends: "bucket"
#   bindings: <
#     role: "role/a"
#     principals: "group:group-a"
#     principals: "group:group-b"
#     principals: "project:proj-a"
#     principals: "project:proj-b"
#     principals: "user:a@example.com"
#     principals: "user:b@example.com"
#   >
#   bindings: <
#     role: "role/b"
#     principals: "group:group-a"
#     principals: "group:group-b"
#     principals: "project:proj-a"
#     principals: "project:proj-b"
#     principals: "user:a@example.com"
#     principals: "user:b@example.com"
#   >
# >
# realms: <
#   name: "realm2"
#   extends: "bucket"
#   extends: "realm1"
#   bindings: <
#     role: "role/a"
#     principals: "group:group-a"
#     principals: "group:group-b"
#     principals: "project:proj-a"
#     principals: "project:proj-b"
#     principals: "user:a@example.com"
#     principals: "user:b@example.com"
#   >
#   bindings: <
#     role: "role/b"
#     principals: "group:group-a"
#     principals: "group:group-b"
#     principals: "project:proj-a"
#     principals: "project:proj-b"
#     principals: "user:a@example.com"
#     principals: "user:b@example.com"
#   >
#   bindings: <
#     role: "role/c"
#     principals: "group:group-c"
#     principals: "project:proj-c"
#     principals: "user:c@example.com"
#   >
# >
# ===
