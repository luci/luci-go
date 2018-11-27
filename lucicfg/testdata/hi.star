def test():
  test_works()
  test_should_fail()

def test_works():
  say_hi('Hello!')

def test_should_fail():
  assert.fails(lambda: say_hi(None), 'got NoneType, want string')
