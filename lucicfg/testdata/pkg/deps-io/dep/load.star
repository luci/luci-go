_pkg = lucicfg.current_module().package

# This also technically works, since when loading we can resolve
# relative paths correctly (as relative to the module being loaded).
#
# But exported functions are called from other modules and they must
# use fully-qualified references in order to read files from the
# current package.
_ = io.read_file("res.cfg")

def say_hi():
  lucicfg.emit(
      dest = "load.cfg",
      data = io.read_file("%s//res.cfg" % _pkg)
  )
