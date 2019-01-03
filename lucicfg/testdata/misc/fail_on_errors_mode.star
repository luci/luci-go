load("@stdlib//internal/error.star", "error")

__native__.fail_on_errors()
assert.fails(lambda: error("boo"), "boo")

# No errors are emitted globally, as checked by assertion below.

# Expect errors:
