pkg.declare(
    name = "@lucicfg/tests",
    lucicfg = "1.1.2",  # the mocked "1.1.1" is older than that => error
)
pkg.entrypoint("main.star")
