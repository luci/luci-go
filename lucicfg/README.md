# LUCI Configuration Generator

`lucicfg` is a tool for generating low-level LUCI configuration files based on a
high-level configuration given as a [Starlark] script that uses APIs exposed by
`lucicfg`. In other words, it takes a \*.star file (or files) as input and spits
out a bunch of \*.cfg files (such us `cr-buildbucket.cfg` and
`luci-scheduler.cfg`) as outputs.

[Starlark]: https://github.com/google/starlark-go

## Overview of the design

`lucicfg` follows a "microkernel" architecture. The kernel is implemented in Go.
It provides a private interface (used internally by the lucicfg's Starlark
standard library) by registering a bunch of builtins. The functionality provided
by the kernel is pretty generic:

*   A notion of Starlark packages, `load(...)` and `exec(...)` implementation.
*   `lucicfg.var(...)` implementation.
*   A node graph structure to carry the state across module boundaries.
*   Support for traversing the node graph and emitting files to an output.
*   Support for Protobuf messages.
*   Implementation of `lucicfg generate` and `lucicfg validate` logic.
*   Various utilities (regexp, hashes, Go templates, etc.)

The builtins are wrapped in two layers of Starlark code: *
[starlark/stdlib/internal], excluding `.../luci`: generic (not LUCI specific)
Starlark standard library that documents and exposes the builtins via a nicer
API. It can be used to build all kinds of config generators ([1], [2], [3]), as
well as extend the LUCI config generator. This API surface is currently marked
as "internal" (meaning there's no backward compatibility guarantees for it), but
it will some day become a part of lucicfg's public interface, so it should be
treated as such (no hacks, no undocumented functions, adequate test coverage,
etc). * [starlark/stdlib/internal/luci]: all LUCI-specific APIs and
declarations, implementing the logic of generating LUCI configs specifically. It
is built entirely on top of the standard library.

The standard library and LUCI configs library are bundled with `lucicfg` binary
via starlark/assets.gen.go file generated from the contents of
[starlark/stdlib/internal] by `go generate`.

[starlark/stdlib/internal]: ./starlark/stdlib/internal
[starlark/stdlib/internal/luci]: ./starlark/stdlib/internal/luci
[1]: https://chrome-internal.googlesource.com/infradata/config/+/refs/heads/main/starlark/common/lib
[2]: https://chrome-internal.googlesource.com/infradata/k8s/+/refs/heads/main/starlark/lib
[3]: https://chrome-internal.googlesource.com/infradata/gae/+/refs/heads/main/starlark/lib

## Making changes to the Starlark portion of the code

1.  Modify a `*.star` file.
2.  In the `lucicfg` directory (where this README.md file is) run
    `./fmt-lint.sh` to auto-format and lint new code. Fix all linter warnings.
3.  In the same directory run `go generate ./...` to regenerate
    `examples/.../generated` and [doc/README.md].
4.  Run `go test ./...` to verify existing tests pass. If your change modifies
    the format of emitted files you need to update the expected output in test
    case files. It will most likely happen for [testdata/full_example.star].
    Update `Expect configs:` section there.
5.  If your change warrants a new test, add a file somewhere under [testdata/].
    See existing files there for examples. There are two kinds of tests:
    *   "Expectation style" tests. They have `Expect configs:` or `Expect errors
        like:` sections at the bottom. The test runner will execute the Starlark
        code and compare the produced output (or errors) to the expectations.
    *   More traditional unit tests that use `assert.eq(...)` etc. See
        [testdata/misc/version.star] for an example.
6.  Once you are done with the change, evaluate whether you need to bump lucicfg
    version. See the section below.

[doc/README.md]: ./doc/README.md
[testdata/full_example.star]: ./testdata/full_example.star
[testdata/]: ./testdata
[testdata/misc/version.star]: ./testdata/misc/version.star

## Updating lucicfg version

`lucicfg` uses a variant of semantic versioning to identify its own version and
a version of the bundled Starlark libraries. The version string is set in
[version.go] and looks like `MAJOR.MINOR.PATCH`.

If a user script is relying on a feature which is available only in some recent
lucicfg version, it can perform a check, like so:

```starlark
lucicfg.check_version('1.7.8', 'Please update depot_tools')
```

That way if the script is executed by an older lucicfg version, the user will
get a nice actionable error message instead of some obscure stack trace.

Thus it is **very important** to update [version.go] before releasing changes:

*   Increment `PATCH` version when you make backward compatible changes. A
    change is backward compatible if it doesn't reduce Starlark API surface and
    doesn't affect lucicfg's emitted output (assuming inputs do not change). In
    particular, adding a new feature that doesn't affect existing features is
    backward compatible.
*   Increment `MINOR` version when you make backward incompatible changes.
    Releasing such changes may require modifying user scripts or asking users to
    regenerate their configs to get an updated output. Note that both these
    things are painful, since there are dozens of repositories with lucicfg
    scripts and, strictly speaking, all of them should be eventually updated.
*   `MAJOR` version is reserved for major architecture changes or rewrites.

If your new feature is experimental and you don't want to commit to any backward
compatibility promises, hide it behind an experiment. Users will need to opt-in
to use it. See [starlark/stdlib/internal/experiments.star] for more info.

[version.go]: ./version.go
[starlark/stdlib/internal/experiments.star]: ./starlark/stdlib/internal/experiments.star

## Making a release

1.  Land the code change. For concreteness sake let's assume it resulted in this
    [luci-go commit].
2.  Wait until it is rolled into [infra.git]. The roll will look like this
    [infra.git commit]. Notice its git hash `86afde8bddae...`.
3.  Wait until the [CIPD package builders] produce per-platform lucicfg
    [CIPD packages] tagged with infra.git's hash `git_revision:86afde8bddae...`.
    Like [this one].
4.  Land a depot_tools CL to release the new version to developer workstations:
    1.  Modify [cipd_manifest.txt]: `infra/tools/luci/lucicfg/${platform}
        git_revision:86afde8bddae...`.
    2.  As instructed in the comments, regenerate cipd_manifest.versions: `cipd
        ensure-file-resolve -ensure-file cipd_manifest.txt`.
    3.  Send a CL [like this], describing in the commit message what's new.
5.  Modify [cr-buildbucket/settings.cfg] like [so] to release the change to
    bots. This step is necessary since bots don't use depot_tools.

Steps 2 and 3 usually take about 30 minutes total, and steps 4 and 5 verify CIPD
packages actually exist. So in practice it is OK to just land a lucicfg CL, go
do other things, then come back >30 min later, look up the revision of the
necessary infra.git DEPS roll (or just use the latest one) and proceed to steps
4 and 5.

[infra.git]: https://chromium.googlesource.com/infra/infra/
[luci-go commit]: https://chromium.googlesource.com/infra/luci/luci-go.git/+/7ac4bfbe5a282766ea2e8afa5a6a06e8b71879f3
[infra.git commit]: https://chromium.googlesource.com/infra/infra/+/86afde8bddaefce47381b7cc4638b36717803d3a
[CIPD package builders]: https://ci.chromium.org/p/infra-internal/g/infra-packagers/console
[CIPD packages]: https://chrome-infra-packages.appspot.com/p/infra/tools/luci/lucicfg
[this one]: https://chrome-infra-packages.appspot.com/p/infra/tools/luci/lucicfg/linux-amd64/+/git_revision:86afde8bddaefce47381b7cc4638b36717803d3a
[cipd_manifest.txt]: https://chromium.googlesource.com/chromium/tools/depot_tools/+/refs/heads/main/cipd_manifest.txt
[like this]: https://chromium-review.googlesource.com/c/chromium/tools/depot_tools/+/2137983
[cr-buildbucket/settings.cfg]: https://chrome-internal.googlesource.com/infradata/config/+/refs/heads/main/configs/cr-buildbucket/settings.cfg
[so]: https://chrome-internal-review.googlesource.com/c/infradata/config/+/2849250
