core.project(
    name = 'proj',
    buildbucket = 'cr-buildbucket.appspot.com',
    swarming = 'chromium-swarm.appspot.com',
)


core.bucket(name = 'b1')
core.bucket(name = 'b2')


core.builder(
    name = 'b1 builder',
    bucket = 'b1',
)
core.builder(
    name = 'ambiguous builder',
    bucket = 'b1',
)
core.builder(
    name = 'b2 builder',
    bucket = 'b2',
)
core.builder(
    name = 'ambiguous builder',
    bucket = 'b2',
)


core.gitiles_poller(
    name = 'valid',
    bucket = 'b1',
    triggers = [
        'b1 builder',
        'b1/b1 builder',  # this is allowed
        'b2 builder',
        'b2/ambiguous builder',
    ],
)

core.gitiles_poller(
    name = 'ambiguous',
    bucket = 'b1',
    triggers = [
        'b1 builder',
        'ambiguous builder',  # error: is it b1 or b2?
    ],
)


core.builder(
    name = 'triggered',
    bucket = 'b1',
    triggered_by = [
        'b1 builder',
        'ambiguous builder',  # error: is it b1 or b2?
    ],
)

# Expect errors like:
#
# Traceback (most recent call last):
#   //testdata/errors/ambiguous_references.star:41: in <toplevel>
#   ...
# Error: ambiguous reference "ambiguous builder" in core.gitiles_poller("b1/ambiguous"), possible variants:
#   core.builder("b1/ambiguous builder")
#   core.builder("b2/ambiguous builder")
#
# Traceback (most recent call last):
#   //testdata/errors/ambiguous_references.star:51: in <toplevel>
#   ...
# Error: ambiguous reference "ambiguous builder" in core.builder("b1/triggered"), possible variants:
#   core.builder("b1/ambiguous builder")
#   core.builder("b2/ambiguous builder")
