pkg.declare(
    name = "@lucicfg/tests/core",
    lucicfg = "1.1.1",  # mocked
)
pkg.resources([
    "**/*.bin",
    "**/*.json",
    "**/*.textpb",
    "**/*.txt",
])
