pkg.declare(
    name = "@lucicfg/tests",
    lucicfg = "1.1.1",  # mocked
)
pkg.entrypoint("main.star")
pkg.depend(
    name = "@lucicfg/overridden-1",
    source = pkg.source.googlesource(
        host = "test-host",
        repo = "overridden-repo-1",
        ref = "test-ref",
        path = ".",
        revision = "v1",  # will be unused (not even accessed) because overridden
    )
)
pkg.depend(
    name = "@lucicfg/remote-1",
    source = pkg.source.googlesource(
        host = "test-host",
        repo = "repo-1",
        ref = "test-ref",
        path = ".",
        revision = "v1", # will be unused, the override requires v2
    )
)
pkg.depend(
    name = "@lucicfg/remote-2",
    source = pkg.source.googlesource(
        host = "test-host",
        repo = "repo-2",
        ref = "test-ref",
        path = ".",
        revision = "v1",
    )
)
