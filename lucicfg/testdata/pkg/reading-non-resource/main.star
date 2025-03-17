_ = io.read_file("resource.txt")

# Expect errors:
#
# Traceback (most recent call last):
#   //main.star: in <toplevel>
#   @stdlib//internal/io.star: in _read_file
# Error in read_file: cannot load //resource.txt: this non-starlark file is not declared as a resource in pkg.resources(...) in PACKAGE.star and cannot be loaded
