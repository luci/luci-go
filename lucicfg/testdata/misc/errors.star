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
#   //testdata/misc/errors.star:14:6: in <toplevel>
#   //testdata/misc/errors.star:4:8: in func1
#   @stdlib//internal/error.star:33:50: in error
# Error: hello world
#
# Traceback (most recent call last):
#   //testdata/misc/errors.star:12:10: in <toplevel>
#   //testdata/misc/errors.star:10:23: in func2
#   //testdata/misc/errors.star:7:20: in capture_stack
#   <builtin>: in stacktrace
# Error: another err
