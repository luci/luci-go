say_hi('Hello!')

assert.fails(lambda: say_hi(None), 'got NoneType, want string')
