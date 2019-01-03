load("@stdlib//internal/error.star", "error")

__native__.fail_on_errors()
assert.fails(lambda: error("boo"), "boo")

# No errors are emitted, as checked by the assertion below.

# Expect errors:
