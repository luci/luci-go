# Prepare CLI vars as:
#
# consumed_var=
# unconsumed_var1=
# unconsumed_var2=

lucicfg.var(expose_as = "consumed_var")

# Expect errors:
#
# value set by "-var unconsumed_var1=..." was not used by Starlark code
#
# value set by "-var unconsumed_var2=..." was not used by Starlark code
