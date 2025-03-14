pkg.declare(
    name = "@lucicfg/tests",
    lucicfg = "1.1.1",  # mocked
)
pkg.entrypoint("main.star")
pkg.options.lint_checks(["default", "+formatting"])
pkg.options.fmt_rules(
    paths = ["somedir"],
    function_args_sort = ["arg1"],
)
