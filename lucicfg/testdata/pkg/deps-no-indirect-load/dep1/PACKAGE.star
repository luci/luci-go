pkg.declare(
    name = "@lucicfg/dep1",
    lucicfg = "1.1.1",  # mocked
)
pkg.depend(
    name = "@lucicfg/dep2",
    source = pkg.source.local(
        path = "../dep2",
    )
)
