lucicfg.enable_experiment('crbug.com/1085650')

luci.project(
    name = 'proj',

    buildbucket = 'cr-buildbucket.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)

luci.bucket(
    name = 'bucket',
)

luci.realm(
    name = 'realm1',
    extends = 'bucket',
)

luci.realm(
    name = 'realm2',
    extends = ['bucket', 'realm1', '@root'],
)

luci.realm(
    name = '@legacy',
    extends = '@root',
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
# >
# realms: <
#   name: "@root"
# >
# realms: <
#   name: "bucket"
# >
# realms: <
#   name: "realm1"
#   extends: "bucket"
# >
# realms: <
#   name: "realm2"
#   extends: "bucket"
#   extends: "realm1"
# >
# ===
