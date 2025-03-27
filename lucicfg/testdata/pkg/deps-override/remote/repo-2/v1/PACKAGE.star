pkg.declare(
    name = "@lucicfg/remote-2",
    lucicfg = "1.1.1",  # mocked
)
# A purely transitive dependency that is overridden.
pkg.depend(
    name = "@lucicfg/overridden-2",
    source = pkg.source.googlesource(
        host = "test-host",
        repo = "overridden-repo-2",
        ref = "test-ref",
        path = ".",
        revision = "v1",  # will be unused (not even accessed) because overridden
    )
)
