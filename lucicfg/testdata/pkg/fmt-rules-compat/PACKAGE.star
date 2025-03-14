pkg.declare(
    name = "@lucicfg/tests",
    lucicfg = "1.1.1",  # mocked
)
pkg.entrypoint("main.star")
pkg.options.fmt_rules(
    paths = ["."],
    function_args_sort = ["arg1"],
)
