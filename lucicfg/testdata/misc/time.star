load('@stdlib//internal/time.star', 'time')

sec = lambda v: time.duration(v * 1000)


def test_duration_type():
  # Basic starlark stuff.
  assert.eq(type(sec(1)), 'duration')
  assert.eq(str(sec(3600)), '1h0m0s')

  # Comparisons.
  assert.true(sec(123) == sec(123))
  assert.true(sec(123) != sec(456))
  assert.true(sec(123) <  sec(456))
  assert.true(sec(456) >  sec(123))
  assert.true(sec(123) <= sec(123))
  assert.true(sec(123) >= sec(123))

  # duration +- duration binary ops.
  assert.eq(sec(1) + sec(2), sec(3))
  assert.eq(sec(2) - sec(1), sec(1))

  # duration */ int binary op.
  assert.eq(sec(1) * 10, sec(10))
  assert.eq(10 * sec(1), sec(10))
  assert.eq(sec(10) / 10, sec(1))

  # duration / duration binary op.
  assert.eq(sec(100) / sec(5), 20)

  # +- unary ops.
  assert.eq(+sec(1), sec(1))
  assert.eq(-sec(1), sec(-1))


def test_type_mismatches():
  # Not compatible with ints, by design.
  assert.fails(lambda: sec(1) + 1, 'unknown binary op')
  assert.fails(lambda: 1 + sec(1), 'unknown binary op')
  assert.fails(lambda: sec(1) > 1, 'not implemented')
  assert.fails(lambda: 1 > sec(1), 'not implemented')

  # Can't multiply two durations.
  assert.fails(lambda: sec(1) * sec(2), 'unknown binary op')

  # Can't divide ints by duration.
  assert.fails(lambda: 10 / sec(1), 'unknown binary op')


def test_helpers():
  assert.eq(60 * time.second + time.hour, 61 * time.minute)
  assert.eq(3600 * time.second / time.hour, 1)
  assert.eq(time.second * 3600 / time.hour, 1)
  assert.eq(time.truncate(2 * time.hour + time.minute, time.hour), 2 * time.hour)


test_duration_type()
test_type_mismatches()
test_helpers()
