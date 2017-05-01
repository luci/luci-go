## vpython - simple and easy VirtualEnv Python

`vpython` is a tool, written in Go, which enables the simple and easy invocation
of Python code in [VirtualEnv](https://virtualenv.pypa.io/en/stable/)
environments.

`vpython` is a simple Python bootstrap which (almost) transparently wraps a
Python interpreter invocation to run in a tailored VirtualEnv environment. The
environment is expressed by a script-specific configuration file. This allows
each Python script to trivially express its own package-level dependencies and
run in a hermetic world consisting of just those dependencies.

When invoking such a script via `vpython`, the tool downloads its dependencies
and prepares an immutable VirtualEnv containing them. It then invokes the
script, now running in that VirutalEnv, through the preferred Python
interpreter.

`vpython` does its best not to use hacky mechanisms to achieve this. It uses
an unmodified VirtualEnv package, standard setup methods, and local system
resources. The result is transparent canonical VirtualEnv environment
bootstrapping. `vpython` is also safe for concurrent invocation, using safe
filesystem-level locking to perform any environment setup and management.

`vpython` itself is very fast. The wheel downloads and VirtualEnvs may also be
cached and re-used, optimally limiting the runtime overhead of `vpython` to just
one initial setup per unique environment.

### Setup and Invocation

For the standard case, employing `vpython` is as simple as:

First, create and upload Python wheels for all of the packages that you will
need. This is done in an implementation-specific way (e.g., upload wheels as
packages to CIPD).

Once the packages are available:

* Add `vpython` to `PATH`.
* Write an environment specification naming packages.
* Change tool invocation from `python` to `vpython`.

Using `vpython` offers several benefits to direct Python invocation, especially
when vendoring packages. Notably, with `vpython`:

* It is trivially enables hermetic Python everywhere.
* No `sys.path` manipulation is needed to load vendored or imported packages.
* Any tool can define which package(s) it needs without requiring coordination
  or cooperation from other tools. (Note that the package must be made available
  for download first).
* Adding new Python dependencies to a project is non-invasive and immediate.
* Package downloading and deployment are baked into `vpython` and built on
  fast and secure Google Cloud Platform technologies.
* No more custom bootstraps. Several projects and tools, including multiple
  within the infra code base, have bootstrap scripts that vendor packages or
  mimic a VirtualEnv. These are at best repetitive and, at worst, buggy and
  insecure.
* Depenencies are explicitly stated, not assumed.

### Why VirtualEnv?

VirtualEnv offers several benefits over system Python. Primarily, it is the

By using the same environemnt everywhere, Python invocations become
reproducible. A tool run on a developer's system will load the same versions
of the same libraries as it will on a production system. A production system
will no longer fail because it is missing a package, or because it has the
wrong version.

A direct mechanism for vendoring, `sys.path` manipulation, is nuanced, buggy,
and unsupported by the Python community. It is difficult to get right on all
platforms in all environments for all packages. A notorious example of this is
`protobuf` and other domain-bound packages, which actively fight `sys.path`
inclusion. Using VirtualEnv means that any compliant Python package can
trivially be included into a project.

### Why CIPD?

[CIPD](https://github.com/luci/luci-go/tree/master/cipd) is a cross-platform
service and associated tooling and packages used to securely fetch and deploy
immutable "packages" (~= zip files) into the local file system. Unlike "package
managers" it avoids platform-specific assumptions, executable "hooks", or the
complexities of dependency resolution. `vpython` uses this as a mechanism for
housing and deploying wheels.

infrastructure package deployment system. It is simple, accessible, fast, and
backed by resilient systems such as Google Storage and AppEngine.

Unlike `pip`, a CIPD package is defined by its content, enabling precise package
matching instead of fuzzy version matching (e.g., `numpy >= 1.2`, and
`numpy == 1.2` both can match multiple `numpy` packages in `pip`).

CIPD also supports ACLs, enabling privileged Python projects to easily vendor
sensitive packages.

### Why wheels?

A Python [wheel](https://www.python.org/dev/peps/pep-0427/) is a simple binary
distrubition of Python code. A wheel can be generic (pure Python) or system-
and architecture-bound (e.g., 64-bit Mac OSX).

Wheels are prefered over eggs because they come packaged with compiled binaries.
This makes their deployment simple (unpack via `pip`) and reduces system
requirements and variation, since local compilation is not needed.

The increased management burden of maintaining separate wheels for the same
package, one for each architecture, is handled naturally by CIPD, removing the
only real pain point.

## Wheel Guidance

This section contains recommendations for building or uploading wheel CIPD
packages, including platform-specific guidance.

CIPD wheel packages are CIPD packages that contain Python wheels. A given CIPD
package can contain multiple wheels for multiple platforms, but should only
contain one version of any given package for any given architecture/platform.

For example, you can bundle a Windows, Linux, and Mac OSX version of `numpy` and
`coverage` in the same CIPD package, but you should not bundle `numpy==1.11` and
`numpy==1.12` in the same package.

The reason for this is that `vpython` identifies which wheels to install by
scanning the contents of the CIPD package, and if multiple versions appear,
there is no clear guidance about which should be used.

### Mac OSX

Use the `m` ABI suffix and the `macosx_...` platform. `vpython` installs wheels
with the `--force` flag, so slight binary incompatibilities (e.g., specific OSX
versions) can be glossed over.

    coverage-4.3.4-cp27-cp27m-macosx_10_10_x86_64.whl

### Linux

Use wheels with the `mu` ABI suffix and the `manylinux1` platform. For example:

    coverage-4.3.4-cp27-cp27mu-manylinux1_x86_64.whl

### Windows

Use wheels with the `cp27m` or `none` ABI tag. For example:

    coverage-4.3.4-cp27-cp27m-win_amd64.whl


## Setup and Invocation

`vpython` can be invoked by replacing `python` in the command-line with
`vpython`.

`vpython` works with a default Python environment out of the box. To add
vendored packges, you need to define an environment specification file that
describes which wheels to install.

An environment specification file is a text protobuf defined as `Spec`
[here](./api/env/spec.proto). An example is:

```
# Any 2.7 interpreter will do.
python_version: "2.7"

# Include "numpy" for the current architecture.
wheel {
  name: "infra/python/wheels/numpy/${platform}-${arch}"
  version: "version:1.11.0"
}

# Include "coverage" for the current architecture.
wheel {
  name: "infra/python/wheels/coverage/${platform}-${arch}"
  version: "version:4.1"
}
```

This specification can be supplied in one of three ways:

* Explicitly, as a command-line option to `vpython` (`-spec`).
* Implicitly, as a file alongside your entry point. For example, if you are
  running `test_runner.py`, `vpython` will look for `test_runner.py.vpython`
  next to it and load the environment from there.
* Implicitly, inined in your main file. `vpython` will scan the main entry point
  for sentinel text and, if present, load the specification from that.
* Implicitly, through the `VPYTHON_VENV_SPEC_PATH` environment variable. This is
  set by a `vpython` invocation so that chained invocations default to the same
  environment.

### Optimization and Caching

`vpython` has several levels of caching that it employs to optimize setup and
invocation overhead.

#### VirtualEnv

Once a VirtualEnv specification has been resolved, its resulting pinned
specification is hashed and used as a key to that VirtualEnv. Other `vpython`
invocations expressing hte same environment will naturally re-use that
VirtualEnv instead of creating their own.

#### Download Caching

Download mechanisms (e.g., CIPD) can optionally include a package cache to avoid
the overhead of downloading and/or resolving a package multiple times.

### Migration

#### Command-line.

`vpython` is a natural replacement for `python` in the command line:

```sh
python ./foo/bar/baz.py -d --flag value arg arg whatever
```

Becomes:
```sh
vpython ./foo/bar/baz.py -d --flag value arg arg whatever
```

The `vpython` tool accepts its own command-line arguments. In this case, use
a `--` seprator to differentiate between `vpython` options and `python` options:

```sh
vpython -spec /path/to/spec.vpython -- ./foo/bar/baz.py
```

#### Shebang (POSIX)

If your script uses implicit specification (file or inline), replacing `python`
with `vpython` in your shebang line will automatically work.

```sh
#!/usr/bin/env vpython
```

