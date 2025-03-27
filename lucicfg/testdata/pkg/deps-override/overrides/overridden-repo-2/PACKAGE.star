pkg.declare(
    name = "@lucicfg/overridden-2",
    lucicfg = "1.1.1",  # mocked
)
# Local dependency of an override.
pkg.depend(
    name = "@lucicfg/local",
    source = pkg.source.local(
        path = "local",
    ),
)
