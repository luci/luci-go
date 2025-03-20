pkg.declare(
    name = "@lucicfg/local",
    lucicfg = "1.1.1",  # mocked
)
pkg.depend(
    name = "@lucicfg/remote",
    source = pkg.source.googlesource(
        host = "ignored-in-tests",
        repo = "repo",
        ref = "ignored-in-tests",
        path = ".",
        revision = "v2",
    )
)
