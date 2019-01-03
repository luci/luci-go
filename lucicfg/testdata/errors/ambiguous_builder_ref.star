load('@stdlib//internal/generator.star', 'generator')
load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/luci/common.star', 'builder_ref', 'keys')


# TODO(vadimsh): Replace this with something real once we have nodes that refer
# to builders.
def custom_node(name, builders):
  k = graph.key('custom', name)
  graph.add_node(k)
  for b in builders:
    graph.add_edge(k, keys.builder_ref(b))


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


custom_node(
    name = 'valid',
    builders = [
        'b1 builder',
        'b1/b1 builder',  # this is allowed
        'b2 builder',
        'b2/ambiguous builder',
    ],
)


custom_node(
    name = 'ambiguous',
    builders = [
        'b1 builder',
        'ambiguous builder',
    ],
)

################################################################################

load('@stdlib//internal/luci/generators.star', 'get_builders')


def visit_valid_node(ctx):
  valid = graph.node(graph.key('custom', 'valid'))
  assert.eq(get_builders(valid), [
      graph.node(keys.builder('b1/b1 builder')),
      graph.node(keys.builder('b2/ambiguous builder')),
      graph.node(keys.builder('b2/b2 builder')),
  ])
generator(impl = visit_valid_node)


def visit_ambiguous_node(ctx):
  ambiguous = graph.node(graph.key('custom', 'ambiguous'))
  assert.eq(get_builders(ambiguous), [
      # Skipped the ambiguous reference, but generated the error, see below.
      graph.node(keys.builder('b1/b1 builder')),
  ])
generator(impl = visit_ambiguous_node)


# Expect errors:
#
# Traceback (most recent call last):
#   //testdata/errors/ambiguous_builder_ref.star:54: in <toplevel>
#   //testdata/errors/ambiguous_builder_ref.star:10: in custom_node
# Error: ambiguous reference "ambiguous builder" in custom("ambiguous"), possible variants:
#   core.builder("b1/ambiguous builder")
#   core.builder("b2/ambiguous builder")
