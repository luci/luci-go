pkg.declare(
    name = "@lucicfg/tests",
    lucicfg = "1.1.1",  # mocked
)
pkg.entrypoint("main.star")
pkg.depend(
    name = "@lucicfg/dep",
    source = pkg.source.local(
        path = "dep",
    )
)
