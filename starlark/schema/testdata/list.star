# Tests for list validators.


def default_is_empty_list():
  val, err = schema.validate(None, [schema.int()])
  assert.eq(val, [])
  assert.eq(err, None)
default_is_empty_list()


def success_with_list():
  val, err = schema.validate([123, 456], [schema.int()])
  assert.eq(val, [123, 456])
  assert.eq(err, None)
success_with_list()


def success_with_tuple():
  val, err = schema.validate((123, 456), [schema.int()])
  assert.eq(val, [123, 456])
  assert.eq(err, None)
success_with_tuple()


def wrong_type():
  val, err = schema.validate(123, [schema.int()])
  assert.eq(val, None)
  assert.eq(err, 'got int (123), expecting a list or a tuple')
wrong_type()


def wrong_element():
  val, err = schema.validate([123, 'zzz'], [schema.int()])
  assert.eq(val, None)
  assert.eq(err, 'in [1]: got string (\"zzz\"), expecting int')
wrong_element()


def bad_list_schema():
  val, err = schema.validate([123], [schema.int(), schema.int()])
  assert.eq(val, None)
  assert.eq(err, 'bad list schema: expecting exactly one item with elements schema')
bad_list_schema()
