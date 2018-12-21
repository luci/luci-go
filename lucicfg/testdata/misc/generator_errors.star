def gen1(ctx):
  fail("gen1 failed")
core.generator(impl = gen1)

trace = stacktrace()

def gen2(ctx):
  fail("gen2 failed", trace=trace)
core.generator(impl = gen2)

# Expect errors:
#
# Traceback (most recent call last):
#   //testdata/misc/generator_errors.star:2: in gen1
# Error: gen1 failed
#
# Traceback (most recent call last):
#   //testdata/misc/generator_errors.star:5: in <toplevel>
# Error: gen2 failed
