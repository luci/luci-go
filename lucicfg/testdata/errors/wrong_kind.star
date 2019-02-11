luci.project(
    name = 'proj',
    buildbucket = 'cr-buildbucket.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)

luci.builder(
    name = 'builder',
    bucket = luci.recipe(
        name = 'noop',
        cipd_package = 'noop',
    ),
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/wrong_kind.star:7: in <toplevel>
#   @stdlib//internal/luci/rules/builder.star:165: in builder
#   ...
# Error: expecting luci.bucket, got luci.recipe
