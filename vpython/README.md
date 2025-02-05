[TOC]

## vpython - simple and easy Virtualenv Python

`vpython` is a tool, written in Go, which enables the simple and easy invocation
of Python code in [Virtualenv](https://virtualenv.pypa.io/en/stable/)
environments.

`vpython` is a simple Python bootstrap which (almost) transparently wraps a
Python interpreter invocation to run in a tailored Virtualenv environment. The
environment is expressed by a script-specific configuration file. This allows
each Python script to trivially express its own package-level dependencies and
run in a hermetic world consisting of just those dependencies.

When invoking such a script via `vpython`, the tool downloads its dependencies
and prepares an immutable Virtualenv containing them. It then invokes the
script, now running in that Virtualenv, through the preferred Python
interpreter.

`vpython` does its best not to use hacky mechanisms to achieve this. It uses
an unmodified Virtualenv package, standard setup methods, and local system
resources. The result is transparent canonical Virtualenv environment
bootstrapping that meets the expectations of standard Python packages. `vpython`
is also safe for concurrent invocation, using safe filesystem-level locking to
perform any environment setup and management.

`vpython` itself is very fast. The wheel downloads and Virtualenvs may also be
cached and re-used, optimally limiting the runtime overhead of `vpython` to just
one initial setup per unique environment.

### Setup and Invocation

For the standard case, employing `vpython` is as simple as:

1. Create a `vpython` Virtualenv specification (or don't, if no additional
   packages are needed.
2. Invoke your script through `vpython` instead of `python`.

If additional Python libraries are needed, you may create new packages for those
libraries. This is done in an implementation-specific way (e.g., upload wheels
as packages to CIPD).

Once the packages are available:

* Add `vpython` to `PATH`.
* Write an environment specification naming packages.
* Change tool invocation from `python` to `vpython`.

Using `vpython` offers several benefits to direct Python invocation, especially
when vendoring packages. Notably, with `vpython`:

* It trivially enables hermetic Python everywhere, greatly increasing control
  and removing per-system differences in Python packages and environment.
* It handles situations that system-level packages cannot accommodate, such as
  different scripts with different versions of packages running in them.
* No `sys.path` manipulation is needed to load vendored or imported packages.
* Any tool can define which package(s) it needs without requiring coordination
  or cooperation from other tools. (Note that the package must be made available
  for download first).
* Adding new Python dependencies to a project is non-invasive and immediate.
* Package downloading and deployment are baked into `vpython` and built on
  fast and secure Google Cloud Platform technologies.
* No more custom bootstraps. Several projects and tools, including multiple
  places within Chrome's infra code base, have bootstrap scripts that vendor
  packages or mimic a Virtualenv. These are at best repetitive and, at worst,
  buggy and insecure.
* Dependencies are explicitly stated, not assumed, and consistent between
  deployments.

### Why Virtualenv?

Virtualenv offers several benefits over system Python. Primarily, it is the
*de facto* encapsulated environment method used by the Python community and is
generally used as the standard for a functional deployable package.

By using the same environment everywhere, Python invocations become
reproducible. A tool run on a developer's system will load the same versions
of the same libraries as it will on a production system. A production system
will no longer fail because it is missing a package, because it has the
wrong version of that package, or because a package is incompatible with another
installed package.

A direct mechanism for vendoring, `sys.path` manipulation, is nuanced, buggy,
and unsupported by the Python community. It is difficult to do correctly on all
platforms in all environments for all packages. A notorious example of this is
`protobuf` and other domain-bound packages, which actively fight `sys.path`
inclusion and require special non-intuitive hacks to work. Using Virtualenv
means that any compliant Python package can trivially be included into a
project.

### Why CIPD?

[CIPD](https://github.com/luci/luci-go/tree/master/cipd) is a cross-platform
service and associated tooling and packages used to securely fetch and deploy
immutable "packages" (~= zip files) into the local file system. Unlike package
managers, it avoids platform-specific assumptions, executable hooks, or the
complexities of dependency resolution. `vpython` uses this as a mechanism for
housing and deploying wheels.

Unlike `pip`, a CIPD package is defined by its content, enabling precise package
matching instead of fuzzy version matching (e.g., `numpy >= 1.2`, and
`numpy == 1.2` both can match multiple `numpy` packages in `pip`).

CIPD also supports ACLs, enabling privileged Python projects to easily vendor
sensitive packages.

### Why wheels?

A Python [wheel](https://www.python.org/dev/peps/pep-0427/) is a simple binary
distribution of Python code. A wheel can be generic (pure Python) or system-
and architecture-bound (e.g., 64-bit Mac OSX).

Wheels are preferred over Python eggs because they come packaged with compiled
binaries. This makes their deployment fast and simple: unpack via `pip`. It also
reduces system requirements and variation, since local compilation, headers,
and build tools are not enlisted during installation.

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

## Setup and Invocation

`vpython` can be invoked by replacing `python3` in the command-line with
`vpython3`.

`vpython` works with a default Python environment out of the box. To add
vendored packages, you need to define an environment specification file that
describes which wheels to install.

An environment specification file is a text protobuf defined as `Spec`
[here](./api/vpython/spec.proto). An example is:

```
# Any 3.11 interpreter will do.
python_version: "3.11"

# Include "cffi" for the current architecture.
wheel: <
  name: "infra/python/wheels/cffi/${vpython_platform}"
  version: "version:1.14.5.chromium.7"
>
```

This specification can be supplied in one of four ways:

* Explicitly, as a command-line option to `vpython` (`-vpython-spec`).
* Implicitly, as a file alongside your entry point. For example, if you are
  running `test_runner.py`, `vpython` will look for `test_runner.py.vpython`
  next to it and load the environment from there.
* Implicitly, inlined in your main file. `vpython` will scan the main entry
  point for sentinel text and, if present, load the specification from that.
* Implicitly, through the `VPYTHON_DEFAULT_SPEC` environment variable.

### Optimization and Caching

`vpython` has several levels of caching that it employs to optimize setup and
invocation overhead.

#### Virtualenv

Once a Virtualenv specification has been resolved, its resulting pinned
specification is hashed and used as a key to that Virtualenv. Other `vpython`
invocations expressing the same environment will naturally re-use that
Virtualenv instead of creating their own.

#### Download Caching

Download mechanisms (e.g., CIPD) can optionally include a package cache to avoid
the overhead of downloading and/or resolving a package multiple times.

### Migration

#### Command-line.

`vpython3` is a natural replacement for `python3` in the command line:

```sh
python3 ./foo/bar/baz.py -d --flag value arg arg whatever
```

Becomes:
```sh
vpython3 ./foo/bar/baz.py -d --flag value arg arg whatever
```

The `vpython` tool accepts its own command-line arguments. In this case, use
a `--` separator to differentiate between `vpython` options and `python` options:

```sh
vpython3 -vpython-spec /path/to/spec.vpython -- ./foo/bar/baz.py
```

#### Shebang (POSIX)

If your script uses implicit specification (file or inline), replacing `python`
with `vpython` in your shebang line will automatically work.

```sh
#!/usr/bin/env vpython3
```

## Configuration

There are a number of environment variables that can affect vpython's behavior.
These are the following:

*   `VPYTHON_BYPASS`: If set to `manually managed python not supported by chrome
    operations`, vpython will do nothing and will instead directly invoke the
    next `python` on PATH. Will have no effect if it's set to anything else.
*   `VPYTHON_DEFAULT_SPEC`: Specifies path to a vpython spec file that will be
    used if none is provided or found through probing.
*   `VPYTHON_LOG_TRACE`: Specifies log level of vpython. Can also be specified
    via the "-vpython-log-level" cmd-line flag.
*   `VPYTHON_VIRTUALENV_ROOT`: Specifies the VirtualEnv root. Default is
    `~/.vpython-root`. Can also be specified via the "-vpython-root" cmd-line
    flag.
