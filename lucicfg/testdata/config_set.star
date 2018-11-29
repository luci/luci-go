load("@proto//test.proto", "testproto")


def test_config_set():
  cs = __native__.new_config_set()
  assert.eq(type(cs), 'config_set')

  # Setters work.
  cs['hello'] = 'world'
  cs['proto'] = testproto.Msg(i=123)
  assert.eq(cs['hello'], 'world')
  assert.eq(cs['proto'].i, 123)

  # Only string keys are allowed.
  def try_set():
    cs[1] = '...'
  assert.fails(try_set, 'should be a string, not int')

  # Only string or proto values are allowed.
  def try_set():
    cs['1'] = 1
  assert.fails(try_set, 'should be either a string or a proto message, not int')


def test_config_generators():
  def gen1(ctx):
    ctx.config_set['hello'] = 'world'
  core.generator(impl = gen1)

  def gen2(ctx):
    assert.eq(ctx.config_set['hello'], 'world')
    ctx.config_set['hello'] = 'nope'
  core.generator(impl = gen2)

  ctx = __native__.new_gen_ctx()
  __native__.call_generators(ctx)
  assert.eq(ctx.config_set['hello'], 'nope')


def with_clean_state(cb):
  __native__.clear_state()
  cb()
  __native__.clear_state()


with_clean_state(test_config_set)
with_clean_state(test_config_generators)
