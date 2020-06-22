luci.project(name = "test")

lucicfg.emit(
    dest = "from-exec",
    data = str(lucicfg.current_module()) + "\n",
)

# Expect configs:
#
# Nothing. Must not be executed as a separate test case, since it is under
# 'support' skipped by TestAllStarlark in starlark_test.go.
