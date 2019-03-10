load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/lucicfg.star', 'lucicfg')

key = graph.key('kind', 'id')

def _some_rule(ctx, arg, *, kwarg):
  assert.eq(arg, 'arg')
  assert.eq(kwarg, 'kwarg')
  return graph.keyset(key)
some_rule = lucicfg.rule(impl = _some_rule)

assert.eq(str(some_rule), '<rule some_rule>')
assert.eq(some_rule('arg', kwarg='kwarg').get('kind'), key)

broken_rule = lucicfg.rule(impl = lambda ctx: 123)
assert.fails(lambda: broken_rule(), 'must return graph.keyset, got int')
