pkg.declare(
    name = "@lucicfg/dep",
    lucicfg = "1.1.1",  # mocked
)
pkg.depend(
    name = "@lucicfg/tests",
    source = pkg.source.local(
        path = "..",
    )
)
