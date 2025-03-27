pkg.declare(
    name = "@lucicfg/overridden-1",
    lucicfg = "1.1.1",  # mocked
)
pkg.depend(
    name = "@lucicfg/remote-1",
    source = pkg.source.googlesource(
        host = "test-host",
        repo = "repo-1",
        ref = "test-ref",
        path = ".",
        revision = "v2",
    )
)
