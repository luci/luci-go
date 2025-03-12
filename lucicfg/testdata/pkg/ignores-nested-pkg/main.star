exec("//good/script.star")
exec("//bad/script.star")

# Expect errors:
#
# Traceback (most recent call last):
#   //main.star: in <toplevel>
# Error in exec: cannot exec //bad/script.star: directory "bad" belongs to a different (nested) package and files from it cannot be loaded directly
