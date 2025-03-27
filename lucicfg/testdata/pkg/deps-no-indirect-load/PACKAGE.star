pkg.declare(
    name = "@lucicfg/tests",
    lucicfg = "1.1.1",  # mocked
)
pkg.entrypoint("main.star")
pkg.depend(
    name = "@lucicfg/dep1",
    source = pkg.source.local(
        path = "dep1",
    )
)
# No direct dependency on dep2.
