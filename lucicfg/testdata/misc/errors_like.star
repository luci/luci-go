load("@stdlib//internal/error.star", "error")

def func1():
  error("hello %s", "world")

def capture_stack():
  return stacktrace()

def func2():
  return capture_stack()

s = func2()

func1()
error("another err", stack=s)

# Expect errors like:
#
# Traceback (most recent call last):
#   ...
# Error: hello world
#
# Traceback (most recent call last):
#   ...
#   //testdata/misc/errors_like.star:???: in func2
#   //testdata/misc/errors_like.star:???: in capture_stack
# Error: ??? err
