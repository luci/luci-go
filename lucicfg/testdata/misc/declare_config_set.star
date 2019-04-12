def test_ok():
  ctx = __native__.new_gen_ctx()
  ctx.declare_config_set('testing/1', '.')
  ctx.declare_config_set('testing/1', '.')  # redeclaring is OK
  ctx.declare_config_set('testing/1', 'subdir/..')  # like this too
  ctx.declare_config_set('testing/2', '.')  # intersecting sets are OK
  ctx.declare_config_set('testing/3', 'subdir')  # nested is fine


def test_redeclaration():
  ctx = __native__.new_gen_ctx()
  ctx.declare_config_set('testing', 'abc')
  assert.fails(lambda: ctx.declare_config_set('testing', 'def'),
      'set "testing" has already been declared')


test_ok()
test_redeclaration()
