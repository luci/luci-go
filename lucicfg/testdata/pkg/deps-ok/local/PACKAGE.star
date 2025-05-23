pkg.declare(
    name = "@lucicfg/local",
    lucicfg = "1.1.1",  # mocked
)
pkg.depend(
    name = "@lucicfg/remote",
    source = pkg.source.googlesource(
        host = "test-host",
        repo = "repo",
        ref = "test-ref",
        path = ".",
        revision = "v2",
    )
)
