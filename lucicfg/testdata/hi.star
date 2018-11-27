say_hi('Hello!')

assert.fails(lambda: say_hi(None), 'got NoneType, want string')

# Just check it doesn't crash for now. More meaningful tests will be added
# later when we have some real functionality that uses the state.
__native__.clear_state()
