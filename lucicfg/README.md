# LUCI Configuration Generator

`lucicfg` is a tool for generating low-level LUCI configuration files based on a
high-level configuration given as a [Starlark] script that uses APIs exposed by
`lucicfg`. In other words, it takes a \*.star file (or files) as input and
spits out a bunch of \*.cfg files (such us `cr-buildbucket.cfg` and
`luci-scheduler.cfg`) as outputs.

[Starlark]: https://github.com/google/starlark-go


## Overview of the design

`lucicfg` follows a "microkernel" architecture. The kernel is implemented in Go.
It provides minimal private interface to lucicfg's Starlark standard library by
registering a bunch of builtins (most of them in `__native__` struct). The
functionality provided by the kernel is pretty generic (nothing LUCI specific):

  * A notion of Starlark packages, `load(...)` and `exec(...)` implementation.
  * `lucicfg.var(...)` implementation.
  * A node graph structure to carry the state across module boundaries.
  * Support for traversing the node graph and emitting files to an output.
  * Support for Protobuf messages.
  * Implementation of `lucicfg generate` and `lucicfg validate` logic.
  * Various utilities (regexp, hashes, Go templates, etc.)

The builtins are wrapped in two layers of Starlark code:
  * [starlark/stdlib/internal/], excluding `.../luci/`: generic (not LUCI
    specific) lucicfg's Starlark standard library that documents and exposes
    the builtins via a nicer API. It can be used to build all kinds of config
    generators ([1], [2], [3]) as well as extend LUCI config generation. This
    API surface is currently marked as "internal" (meaning there's no backward
    compatibility guarantees for it), but it will some day become a part of
    lucicfg's public API.
  * [starlark/stdlib/internal/luci]: all LUCI-specific APIs and declarations,
    implementing the logic of generating LUCI configs specifically. It is built
    entirely on top of the standard library.

The standard library and LUCI configs libraries are bundled with `lucicfg`
binary via `starlark/assets.gen.go` file generated from the contents of
`starlark/stdlib` directory by `go generate`.

[starlark/stdlib/internal/]: ./starlark/stdlib/internal
[starlark/stdlib/internal/luci]: ./starlark/stdlib/internal/luci
[1]: https://chrome-internal.googlesource.com/infradata/config/+/refs/heads/master/starlark/common/lib/
[2]: https://chrome-internal.googlesource.com/infradata/k8s/+/refs/heads/master/starlark/lib/
[3]: https://chrome-internal.googlesource.com/infradata/gae/+/refs/heads/master/starlark/lib


## Making changes to the Starlark portion of the code

1. Modify a `*.star` file.
2. In the `lucicfg` directory (where this `README.md` file is) run
   `go generate ./...` to regenerate `starlark/assets.gen.go`,
   `examples/.../generated` and `doc/README.md`.
3. If `examples` generation failed, you may need to run `go generate ./...`
   **again**: it might have used stale `assets.gen.go`. Rerunning it makes sure
   the generator uses the correct version of the code.
4. Run `go test ./...` to verify existing tests pass. If your change modifies
   the format of emitted files you need to update the expected output in test
   case files. It will most likely happen for [testdata/full_example.star].
   Update `Expect configs:` section there.
5. If your change warrants a new test, add a file somewhere under [testdata/].
   See existing files there for examples. There are two kinds of tests:
   * "Expectation style" tests. They have `Expect configs:` or
     `Expect errors like:` sections at the bottom. The test runner will execute
     the Starlark code and compare the produced output (or errors) to the
     expectations.
   * More traditional unit tests that use `assert.eq(...)` etc. See
     [testdata/misc/version.star] for an example.
6. Once you are done with the change, evaluate whether you need to bump lucicfg
   version. See the section below.

[testdata/full_example.star]: ./testdata/full_example.star
[testdata/]: ./testdata
[testdata/misc/version.star]: ./testdata/misc/version.star


## Updating lucicfg version

...


## Making a release

...
