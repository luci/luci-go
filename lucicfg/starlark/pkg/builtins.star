# Copyright 2025 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Symbols available in the default namespace when loading PACKAGE.star file.

Only these symbols (plus Starlark builtins) are available. load(...) is
forbidden.
"""

def _declare(*, name, lucicfg):
    """Declares a lucicfg package.

    It sets the name the package can be imported as and the minimum version of
    lucicfg interpreter required by the package.

    The name must start with `@` and it must be a slash-separated path, where
    each path component must consists of lower case letters, numbers or the
    symbols `-` or `_`. Each path component must begin with a letter. The total
    length of the name must be less than 300 characters. This is a logical path
    and it has no relation to any file systems paths.

    All statements referring to this package from other packages will use
    this package name, e.g.

    ```
    load("@packagename//path/within/the/package.star", ...)
    ```

    See [Modules and packages](#modules-and-packages) for more details.

    This name should be reasonably unique: it is impossible to use two
    different packages with the same name as dependencies in a single
    dependency tree (even if they are indirect dependencies). See
    [Dependency version resolution](#pkg-deps) for more details.

    For repos containing recipes, this should be the same value as the recipe
    repo-name for consistency, though to avoid recipe<->lucicfg entanglement,
    this is not a requirement.

    This statement is required exactly once and it must be the first statement
    in PACKAGE.star file.

    The names `@stdlib`, `@__main__` and `@proto` are reserved.

    Args:
      name: the name for this package. Required.
      lucicfg: a string `major.minor.revision` with a minimum lucicfg version
        this package requires. Required.
    """
    __native__.declare(
        _validate_string("name", name),
        _validate_string("lucicfg", lucicfg),
    )

def _depend(*, name, source):
    """Declares a dependency on another lucicfg package.

    For remote packages this essentially declares that
    `load("@<name>//<path>", ...)` statements should be resolved to files from
    the given source at a revision no older than the one specified in the
    pkg.source.googlesource(...) declaration (i.e. this declaration puts
    a constraint on acceptable revisions of the dependency).

    The name must match the name declared by the dependent package itself in
    its pkg.declare(...) statement.

    Packages form a dependency graph. All dependencies with a given name should
    all be fetched from the same source (the same repo and the same ref), but
    perhaps have different revision constraints. The final single version
    of the dependency that will be used by all packages in the graph is picked
    as the most recent among all collected revision constraints (using the git
    ref's history for finding which version is the most recent).

    The resolved versions are written into a lock file of the current package
    (`PACKAGE.lock`, which is a JSON file). This lock file is used exclusively
    when this package is the entry point package for some lucicfg execution. In
    other words, lock files of imported dependencies have no effect on the
    dependency resolution process at all.

    The lock file is generated as a side effect of running `lucicfg generate`
    subcommand and it must be checked in into the repository with the rest
    of the package. It is validated as part of `lucicfg validate` call.

    **NOTE**: the lock file is an optimization: it can always be
    deterministically derived from `PACKAGE.star` and the state of git
    repositories with dependencies. And in fact, as of lucicfg v1.45.0, it is
    always recalculated by `lucicfg generate` and is not actually read by
    anything yet. Nevertheless it is essential for some planned workflows that
    can fetch individual Git files by their revisions, but can't do any other
    generic git operations (like comparing revisions).

    Args:
      name: the name of the depended package. Required.
      source: a pkg.source.ref struct as produced by
        pkg.source.googlesource(...) or pkg.source.local(...). Required.
    """
    __native__.depend(
        __native__.stacktrace(1),
        _validate_string("name", name),
        _validate_source_ref("source", source),
    )

def _resources(patterns):
    """Declares non-Starlark files to includes into the package.

    Only these files can be read at runtime via io.read_file(...) or
    io.read_proto(...).

    Declaring them upfront is useful when the package is used as a dependency.
    Resource files are prefetched with the rest of the package source.

    Can be called multiple times. Works additively.

    Args:
      patterns: a list of glob patterns that define a subset of non-Starlark
        files under the package directory. Each entry is either `<glob pattern>`
        (a "positive" glob) or `!<glob pattern>` (a "negative" glob). A file is
        considered to be a resource file if its slash-separated path matches any
        of the positive globs and none of the negative globs. If a pattern
        starts with `**/`, the rest of it is applied to the base name of the
        file (not the whole path). If only negative globs are given, a single
        positive `**/*` glob is implied as well. Required.
    """
    __native__.resources(_validate_str_list("patterns", patterns))

def _entrypoint(path):
    """Declares that the given Starlark file is one of the entry point scripts.

    Entry point scripts are scripts that can be executed (via
    `lucicfg generate <path>`) to generate some configuration file. Only entry
    point scripts can be executed.

    Args:
      path: a path to a Starlark file relative to the package root. Required.
    """
    __native__.entrypoint(_validate_path("path", path))

def _googlesource(*, host, repo, ref, path, revision):
    """Defines a reference to package source stored in a googlesource.com repo.

    Args:
      host: a googlesource.com source host name (e.g. `chromium`). Required.
      repo: a name of the repository on the host (e.g. `chromium/src`).
        Required.
      ref: a full git reference (e.g. `refs/heads/main`) to fetch. The history
        of this reference is used to determine the ordering of commits when
        resolving versions of dependencies. Required.
      path: a directory path to the lucicfg package root (a directory with
        PACKAGE.star file) within the source repo. Required.
      revision: a full git commit hash with a minimum compatible version of this
        dependency. In the final resolved dependency set, the dependency will be
        at this revision or newer (in case some other package depends on a newer
        version). Must be reachable from the given git ref. Required.

    Returns:
      A pkg.source.ref struct that can be passed to pkg.depend(...).
    """
    return _make_source_ref(
        host = _validate_string("host", host),
        repo = _validate_path("repo", repo),
        ref = _validate_string("ref", ref),
        path = _validate_path("path", path),
        revision = _validate_string("revision", revision),
    )

def _local(path):
    """Defines a reference to package source stored in the current repository.

    Works relative to the repository of the package that declared the
    dependency (aka "the current package repository").

    Constructs pkg.source.googlesource(...) by taking the source reference of
    the current package and replacing the path there. Notably, such dependency
    retains the same revision constraints as the package that imported it.

    If a package `X` is imported directly at revision `A`, and also as a
    transitive local dependency of some other package `Y` at revision `B`, it is
    possible the final execution would use `X` at revision `A` and `Y` at
    revision `B` (if `A` is newer than `B`).

    Args:
      path: a relative path from the current package directory to the
        directory (within the same repository) with the target dependency.
        Required.

    Returns:
      A pkg.source.ref struct that can be passed to pkg.depend(...).
    """
    return _make_source_ref(
        local_path = _validate_path("path", path, allow_dots = True),
    )

def _lint_checks(checks):
    """Configures linting rules that apply to files in this package.

    Can be called at most once.

    Args:
      checks: a list of linter checks to apply in `lucicfg validate` and
        `lucicfg lint`. The first entry defines what group of checks to use as
        a base and it can be one of `none`, `default` or `all`. The following
        entries either add checks to the set (`+<name>`) or remove them
        (`-<name>`). See [Formatting and linting Starlark code](#formatting-linting)
        for more info. Required.
    """
    __native__.lint_checks(_validate_str_list("checks", checks))

def _fmt_rules(*, paths, function_args_sort = None):
    """Adds a formatting rule set that applies to some paths in the package.

    When processing files, lucicfg will select a single rule set based on the
    longest matching rule's path prefix. For example, if there are two rule
    sets, one formatting "a" and another formatting "a/folder", then for the
    file "a/folder/file.star", only the second rules set would apply. If NO
    rules set matches the file path, then only default formatting will occur.

    Args:
      paths: forward-slash delimited path prefixes for which this rule set
        applies. Rules with duplicate path values are not permitted (i.e. you
        cannot have two rules with a path of "something", nor can you have the
        path "something" duplicated within a single rule). Required.
      function_args_sort: if set, specifies how to sort keyword argument in
        function calls. Should be a list of strings (perhaps empty). Keyword
        arguments in all function calls will be ordered based on the order in
        this list. Arguments that do not appear in the list, will be sorted
        alphanumerically and put after all arguments in the list. This implies
        that passing an empty list will result in sorting all keyword arguments
        in all function calls alphanumerically. Optional.
    """
    paths = _validate_str_list("paths", paths)
    if not paths:
        fail("paths cannot be empty")
    for i, p in enumerate(paths):
        _validate_path("paths[%d]" % i, p)
    if function_args_sort != None:
        function_args_sort = _validate_str_list("function_args_sort", function_args_sort)
    __native__.fmt_rules(__native__.stacktrace(1), paths, function_args_sort)

pkg = struct(
    declare = _declare,
    depend = _depend,
    resources = _resources,
    entrypoint = _entrypoint,
    source = struct(
        googlesource = _googlesource,
        local = _local,
    ),
    options = struct(
        lint_checks = _lint_checks,
        fmt_rules = _fmt_rules,
    ),
)

### Internals.

# A constructor for pkg.source.ref(...) structs.
_source_ref = __native__.genstruct("pkg.source.ref")

def _make_source_ref(
        *,
        local_path = None,
        host = None,
        repo = None,
        ref = None,
        path = None,
        revision = None):
    """Constructs a new pkg.source.ref(...) struct.

    Assumes field types were validated already.

    Args:
      local_path: is a relative path for local dependencies or None for remote
        dependencies.
      host: a googlesource host for a remote dependency or None for local
        dependencies.
      repo: a git repo on the host for a remote dependency or None for local
        dependencies.
      ref: a git ref in the repo for a remote dependency or None for local
        dependencies.
      path: a path within the git repo for a remote dependency or None for local
        dependencies.
      revision: a full git commit (SHA1) for a remote dependency or None for
        local dependencies.

    Returns:
      A pkg.source.ref(...) struct.
    """
    return _source_ref(
        local_path = local_path,
        host = host,
        repo = repo,
        ref = ref,
        path = path,
        revision = revision,
    )

def _validate_source_ref(attr, val):
    """Validates that `val` is a pkg.source.ref(...) struct.

    Args:
      attr: field name with this value, for error messages.
      val: a value to validate.

    Returns:
      The same value.
    """
    if __native__.ctor(val) != _source_ref:
        fail("bad %r: got %s, want a pkg.source.ref(...) struct" % (attr, type(val)))
    return val

def _validate_string(attr, val, *, allow_empty = False, default = None, required = True):
    """Validates that the value is a string and returns it.

    Args:
      attr: field name with this value, for error messages.
      val: a value to validate.
      allow_empty: if True, accept empty string as valid.
      default: a value to use if 'val' is None, ignored if required is True.
      required: if False, allow 'val' to be None, return 'default' in this case.

    Returns:
      The validated string or None if required is False and default is None.
    """
    if val == None:
        if required:
            fail("missing required field %r" % attr)
        if default == None:
            return None
        val = default

    if type(val) != "string":
        fail("bad %r: got %s, want string" % (attr, type(val)))
    if not allow_empty and not val:
        fail("bad %r: must not be empty" % (attr,))

    return val

def _validate_str_list(attr, val):
    """Validate that the value is a list or tuple of strings.

    Args:
      attr: field name with this value, for error messages.
      val: a value to validate.

    Returns:
      The tuple of strings.
    """
    if type(val) != "list" and type(val) != "tuple":
        fail("bad %s: expecting a list or a tuple, got %s" % (attr, type(val)))
    tup = tuple(val)
    for idx, elem in enumerate(tup):
        if type(elem) != "string":
            fail("bad \"%s[%d]\": got %s, want string" % (attr, idx, type(elem)))
        if elem == "":
            fail("bad \"%s[%d]\": an empty string" % (attr, idx))
    return tup

def _validate_path(attr, val, *, allow_dots = False):
    """Validates `val` is a string that is a slash-separated relative path.

    Will verify it is "clean".

    Args:
      attr: field name with this value, for error messages.
      val: a value to validate.
      allow_dots: if True, allow ".." (i.e. the path may point outside).

    Returns:
      The same value.
    """
    err = __native__.validate_path(_validate_string(attr, val), allow_dots)
    if err:
        fail("bad %r: %s" % (attr, err))
    return val
