core.project(
    name = 'proj',
    buildbucket = 'cr-buildbucket.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)
core.bucket(name = 'b')
core.builder(
    name = 'clashing name',
    bucket = 'b',
)
core.gitiles_poller(
    name = 'clashing name',
    bucket = 'b',
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/poller_builder_clash.star:11: in <toplevel>
#   ...
# Error: core.triggerer("b/clashing name") is redeclared, previous declaration:
# Traceback (most recent call last):
#   //testdata/errors/poller_builder_clash.star:7: in <toplevel>
#   ...
