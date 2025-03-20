pkg.declare(
    name = "@lucicfg/tests",
    lucicfg = "1.1.1",  # mocked
)
pkg.entrypoint("main.star")
pkg.depend(
    name = "@lucicfg/local",
    source = pkg.source.local(
        path = "local",
    )
)
pkg.depend(
    name = "@lucicfg/remote",
    source = pkg.source.googlesource(
        host = "ignored-in-tests",
        repo = "repo",
        ref = "ignored-in-tests",
        path = ".",
        revision = "v1",  # will be unused, since @lucicfg/local wants v2
    )
)
