def say_hi(key):
  lucicfg.emit(dest = "%s.cfg" % key, data = "Hi\n")
