load("@stdlib//internal/error.star", "error")

def func1():
  error("hello %s", "world")

def capture_stack():
  return stacktrace()

def func2():
  return capture_stack()

s = func2()

func1()
error("another err", trace=s)

# Expect errors:
#
# Traceback (most recent call last):
#   //testdata/misc/errors.star:14: in <toplevel>
#   //testdata/misc/errors.star:4: in func1
#   @stdlib//internal/error.star:33: in error
# Error: hello world
#
# Traceback (most recent call last):
#   //testdata/misc/errors.star:12: in <toplevel>
#   //testdata/misc/errors.star:10: in func2
#   //testdata/misc/errors.star:7: in capture_stack
#   <builtin>: in stacktrace
# Error: another err
