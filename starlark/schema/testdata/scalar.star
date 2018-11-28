# Tests for primitive scalar validators.


def success():
  val, err = schema.validate(123, schema.int())
  assert.eq(val, 123)
  assert.eq(err, None)
success()


def success_with_zero_default():
  val, err = schema.validate(None, schema.int())
  assert.eq(val, 0)
  assert.eq(err, None)
success_with_zero_default()


def success_with_custom_default():
  val, err = schema.validate(None, schema.int(default=123))
  assert.eq(val, 123)
  assert.eq(err, None)
success_with_custom_default()


def wrong_type():
  val, err = schema.validate("123", schema.int(default=456))
  assert.eq(val, None)
  assert.eq(err, 'got string ("123"), expecting int')
wrong_type()


def required_is_missing():
  val, err = schema.validate(None, schema.int(required=True))
  assert.eq(val, None)
  assert.eq(err, 'a value is required')
required_is_missing()


def declaration_checks_stuff():
  assert.fails(lambda: schema.int(1), 'not expecting positional arguments')
  assert.fails(lambda: schema.int(z=1), 'unexpected keyword argument')
  assert.fails(lambda: schema.int(default='1'), 'default value has wrong type string')
declaration_checks_stuff()


def misc():
  assert.eq(type(schema.int()), 'schema.validator')
  assert.eq(
    str(schema.string(default='123')),
    'schema.string(required=False, default=\"123\")')
  assert.true(schema.int())
  assert.fails(lambda: {schema.int(): 1}, 'unhashable')
misc()
