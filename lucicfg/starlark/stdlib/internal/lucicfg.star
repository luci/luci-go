# Copyright 2019 The LUCI Authors.
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

"""Core lucicfg-related functions."""

load("@stdlib//internal/error.star", "error")
load("@stdlib//internal/strutil.star", "strutil")

def _version():
    """Returns a triple with lucicfg version: `(major, minor, revision)`."""
    return __native__.version

def _check_version(min, message = None):
    """Fails if lucicfg version is below the requested minimal one.

    Useful when a script depends on some lucicfg feature that may not be
    available in earlier versions. lucicfg.check_version(...) can be used at
    the start of the script to fail right away with a clean error message:

    ```python
    lucicfg.check_version(
        min = "1.30.14",
        message = "Update depot_tools",
    )
    ```

    Or even

    ```python
    lucicfg.check_version("1.30.14")
    ```

    Additionally implicitly auto-enables not-yet-default lucicfg functionality
    released with the given version. That way lucicfg changes can be gradually
    rolled out project-by-project by bumping the version string passed to
    lucicfg.check_version(...) in project configs.

    Args:
      min: a string `major.minor.revision` with minimally accepted version.
        Required.
      message: a custom failure message to show.
    """
    min_ver = strutil.parse_version(min)
    if (__native__.pkg_min_lucicfg and
        min_ver != __native__.pkg_min_lucicfg and
        not __native__.testing_tweaks.skip_package_compat_checks):
        fail(
            ("Version passed to lucicfg.check_version (which is %r) should match the " +
             "lucicfg version in pkg.declare(...) in PACKAGE.star (which is %r). " +
             "Eventually pkg.declare(...) in PACKAGE.star will become authoritative and " +
             "lucicfg.check_version will be retired. Until then the versions must agree. " +
             "Please update lucicfg.check_version(...) call.") % (
                "%d.%d.%d" % min_ver,
                "%d.%d.%d" % __native__.pkg_min_lucicfg,
            ),
        )
    cur_ver = _version()
    if cur_ver < min_ver:
        fail(
            "Your lucicfg version v%s is older than required v%s. %s." % (
                "%d.%d.%d" % cur_ver,
                "%d.%d.%d" % min_ver,
                message or "Please update",
            ),
        )
    __native__.set_min_version_for_experiments(min_ver)

def _config(
        *,
        config_service_host = None,
        config_dir = None,
        tracked_files = None,
        fail_on_warnings = None,
        lint_checks = None):
    r"""Sets one or more parameters for the `lucicfg` itself.

    These parameters do not affect semantic meaning of generated configs, but
    influence how they are generated and validated.

    Each parameter has a corresponding command line flag. If the flag is
    present, it overrides the value set via `lucicfg.config` (if any). For
    example, the flag `-config-service-host <value>` overrides whatever was set
    via `lucicfg.config(config_service_host=...)`.

    `lucicfg.config` is allowed to be called multiple times. The most recently
    set value is used in the end, so think of `lucicfg.config(var=...)` just as
    assigning to a variable.

    Args:
      config_service_host: a hostname of a LUCI Config Service to send
        validation requests to. Default is whatever is hardcoded in `lucicfg`
        binary, usually `config.luci.app`.
      config_dir: a directory to place generated configs into, relative to the
        directory that contains the entry point \*.star file. `..` is allowed.
        If set via `-config-dir` command line flag, it is relative to the
        current working directory. Will be created if absent. If `-`, the
        configs are just printed to stdout in a format useful for debugging.
        Default is "generated".
      tracked_files: a list of glob patterns that define a subset of files under
        `config_dir` that are considered generated. Each entry is either
        `<glob pattern>` (a "positive" glob) or `!<glob pattern>` (a "negative"
        glob). A file under `config_dir` is considered tracked if its
        slash-separated path matches any of the positive globs and none of the
        negative globs. If a pattern starts with `**/`, the rest of it is
        applied to the base name of the file (not the whole path). If only
        negative globs are given, single positive `**/*` glob is implied as
        well. `tracked_files` can be used to limit what files are actually
        emitted: if this set is not empty, only files that are in this set will
        be actually written to the disk (and all other files are discarded).
        This is beneficial when `lucicfg` is used to generate only a subset of
        config files, e.g. during the migration from handcrafted to generated
        configs. Knowing the tracked files set is also important when some
        generated file disappears from `lucicfg` output: it must be deleted from
        the disk as well. To do this, `lucicfg` needs to know what files are
        safe to delete. If `tracked_files` is empty (default), `lucicfg` will
        save all generated files and will never delete any file in this case it
        is responsibility of the caller to make sure no stale output remains).
      fail_on_warnings: if set to True treat validation warnings as errors.
        Default is False (i.e. warnings do not cause the validation to fail).
        If set to True via `lucicfg.config` and you want to override it to False
        via command line flags use `-fail-on-warnings=false`.
      lint_checks: a list of linter checks to apply in `lucicfg validate`. The
        first entry defines what group of checks to use as a base and it can
        be one of `none`, `default` or `all`. The following entries either
        add checks to the set (`+<name>`) or remove them (`-<name>`). See
        [Formatting and linting Starlark code](#formatting-linting) for more
        info. Default is `['none']` for now.
    """
    if config_service_host != None:
        __native__.set_meta("config_service_host", config_service_host)
    if config_dir != None:
        __native__.set_meta("config_dir", config_dir)
    if tracked_files != None:
        __native__.set_meta("tracked_files", tracked_files)
    if fail_on_warnings != None:
        __native__.set_meta("fail_on_warnings", fail_on_warnings)
    if lint_checks != None:
        __native__.set_meta("lint_checks", lint_checks)

def _enable_experiment(experiment):
    """Enables an experimental feature.

    Can be used to experiment with non-default features that may later
    change in a non-backwards compatible way or even be removed completely.
    Primarily intended for lucicfg developers to test their features before they
    are "frozen" to be backward compatible.

    Enabling an experiment that doesn't exist logs a warning, but doesn't fail
    the execution. Refer to the documentation and the source code for the list
    of available experiments.

    Args:
      experiment: a string ID of the experimental feature to enable. Required.
    """
    __native__.enable_experiment(experiment)

def _generator(impl):
    """Registers a generator callback.

    Such callback is called at the end of the config generation stage to
    modify/append/delete generated configs in an arbitrary way.

    The callback accepts single argument `ctx` which is a struct with the
    following fields and methods:

      * **output**: a dict `{config file name -> (str | proto)}`. The callback
        is free to modify `ctx.output` in whatever way it wants, e.g. by adding
        new values there or mutating/deleting existing ones.

      * **declare_config_set(name, root)**: proclaims that generated configs
        under the given root (relative to `config_dir`) belong to the given
        config set. Safe to call multiple times with exact same arguments, but
        changing an existing root to something else is an error.

    DocTags:
      Advanced.

    Args:
      impl: a callback `func(ctx) -> None`.
    """
    __native__.add_generator(impl)

def _emit(*, dest = None, data = None):
    """Tells lucicfg to write given data to some output file.

    In particular useful in conjunction with io.read_file(...) to copy files
    into the generated output:

    ```python
    lucicfg.emit(
        dest = "foo.cfg",
        data = io.read_file("//foo.cfg"),
    )
    ```

    Note that lucicfg.emit(...) cannot be used to override generated files.
    `dest` must refer to a path not generated or emitted by anything else.

    Args:
      dest: path to the output file, relative to the `config_dir` (see
        lucicfg.config(...)). Must not start with `../`. Required.
      data: either a string or a proto message to write to `dest`. Proto
        messages are serialized using text protobuf encoding. Required.
    """
    trace = stacktrace(skip = 2)

    def _emit_data(ctx):
        _, err = __native__.clean_relative_path("", dest, False)
        if err:
            error("%s", err, trace = trace)
            return
        if ctx.output.get(dest) != None:
            error("config file %r is already generated by something else", dest, trace = trace)
            return
        ctx.output[dest] = data

    _generator(impl = _emit_data)

def _current_module():
    """Returns the location of a module being currently executed.

    This is the module being processed by a current load(...) or exec(...)
    statement. It has no relation to the module that holds the top-level stack
    frame. For example, if a currently loading module `A` calls a function in
    a module `B` and this function calls lucicfg.current_module(...), the result
    would be the module `A`, even though the call goes through code in the
    module `B` (i.e. lucicfg.current_module(...) invocation itself resided in
    a function in module `B`).

    Fails if called from inside a generator callback. Threads executing such
    callbacks are not running any load(...) or exec(...).

    Returns:
      A `struct(package='...', path='...')` with the location of the module.
    """
    pkg, path = __native__.current_module()
    return struct(package = pkg, path = path)

# A constructor for lucicfg.var structs.
_var_ctor = __native__.genstruct("lucicfg.var")

def _var(*, default = None, validator = None, expose_as = None):
    """Declares a variable.

    A variable is a slot that can hold some frozen value. Initially this slot is
    usually empty. lucicfg.var(...) returns a struct with methods to manipulate
    it:

      * `set(value)`: sets the variable's value if it's unset, fails otherwise.
      * `get()`: returns the current value, auto-setting it to `default` if it
        was unset.

    Note the auto-setting the value in `get()` means once `get()` is called on
    an unset variable, this variable can't be changed anymore, since it becomes
    initialized and initialized variables are immutable. In effect, all callers
    of `get()` within a scope always observe the exact same value (either an
    explicitly set one, or a default one).

    Any module (loaded or exec'ed) can declare variables via lucicfg.var(...).
    But only modules running through exec(...) can read and write them. Modules
    being loaded via load(...) must not depend on the state of the world while
    they are loading, since they may be loaded at unpredictable moments. Thus
    an attempt to use `get` or `set` from a loading module causes an error.

    Note that functions _exported_ by loaded modules still can do anything they
    want with variables, as long as they are called from an exec-ing module.
    Only code that executes _while the module is loading_ is forbidden to rely
    on state of variables.

    Assignments performed by an exec-ing module are visible only while this
    module and all modules it execs are running. As soon as it finishes, all
    changes made to variable values are "forgotten". Thus variables can be used
    to implicitly propagate information down the exec call stack, but not up
    (use exec's return value for that).

    Generator callbacks registered via lucicfg.generator(...) are forbidden to
    read or write variables, since they execute outside of context of any
    exec(...). Generators must operate exclusively over state stored in the node
    graph. Note that variables still can be used by functions that _build_ the
    graph, they can transfer information from variables into the graph, if
    necessary.

    The most common application for lucicfg.var(...) is to "configure" library
    modules with default values pertaining to some concrete executing script:

      * A library declares variables while it loads and exposes them in its
        public API either directly or via wrapping setter functions.
      * An executing script uses library's public API to set variables' values
        to values relating to what this script does.
      * All calls made to the library from the executing script (or any scripts
        it includes with exec(...)) can access variables' values now.

    This is more magical but less wordy alternative to either passing specific
    default values in every call to library functions, or wrapping all library
    functions with wrappers that supply such defaults. These more explicit
    approaches can become pretty convoluted when there are multiple scripts and
    libraries involved.

    Another use case is to allow parameterizing configs with values passed via
    CLI flags. A string-typed var can be declared with `expose_as=<name>`
    argument, making it settable via `-var <name>=<value>` CLI flag. This is
    primarily useful in conjunction with `-emit-to-stdout` CLI flag to use
    lucicfg as a "function call" that accepts arguments via CLI flags and
    returns the result via stdout to pipe somewhere else, e.g.

    ```shell
    lucicfg generate main.star -var environ=dev -emit-to-stdout all.json | ...
    ```

    **Danger**: Using `-var` without `-emit-to-stdout` is generally wrong, since
    configs generated on disk (and presumably committed into a repository) must
    not depend on undetermined values passed via CLI flags.

    DocTags:
      Advanced.

    Args:
      default: a value to auto-set to the variable in `get()` if it was unset.
      validator: a callback called as `validator(value)` from `set(value)` and
        inside lucicfg.var(...) declaration itself (to validate `default` or a
        value passed via CLI flags). Must be a side-effect free idempotent
        function that returns the value to be assigned to the variable (usually
        just `value` itself, but conversions are allowed, including type
        changes).
      expose_as: an optional string identifier to make this var settable via
        CLI flags as `-var <expose_as>=<value>`. If there's no such flag, the
        variable is auto-initialized to its default value (which must be string
        or None). Variables declared with `expose_as` are not settable via
        `set()` at all, they appear as "set" already the moment they are
        declared. If multiple vars use the same `expose_as` identifier, they
        will all be initialized to the same value.

    Returns:
      A struct with two methods: `set(value)` and `get(): value`.
    """

    # Variables that can be bound to CLI flags are string-value, and thus the
    # default value must also be a string (or be absent).
    if expose_as and not (default == None or type(default) == "string"):
        fail(
            "lucicfg.var declared with expose_as must have a string or None " +
            "default, got %s %s" % (type(default), default),
        )

    # The default value (if any) must pass the validation itself.
    if validator and default != None:
        default = validator(default)

    # Validate the value passed via CLI flag (if any).
    preset_value = None
    if expose_as:
        preset_value = __native__.var_flags.get(expose_as)
        if preset_value == None:
            preset_value = default
        elif validator:
            preset_value = validator(preset_value)

    # This declares the variable and pre-sets it to validated value passed via
    # CLI flags (if expose_as is not None). This also puts the corresponding
    # -var flag in the set of "consumed" flags. At the end of the script
    # execution all -var flags provided on the command line must be consumed
    # (the run fails otherwise).
    var_id = __native__.declare_var(expose_as or "", preset_value)

    return _var_ctor(
        set = lambda v: __native__.set_var(var_id, validator(v) if validator else v),
        get = lambda: __native__.get_var(var_id, default),
    )

def _rule(*, impl, defaults = None):
    """Declares a new rule.

    A rule is a callable that adds nodes and edges to an entity graph. It wraps
    the given `impl` callback by passing one additional argument `ctx` to it (as
    the first positional argument).

    `ctx` is a struct with the following fields:

      * `defaults`: a struct with module-scoped defaults for the rule.

    The callback is expected to return a graph.keyset(...) with the set of graph
    keys that represent the added node (or nodes). Other rules use such keysets
    as inputs.

    DocTags:
      Advanced. RuleCtor.

    Args:
      impl: a callback that actually implements the rule. Its first argument
        should be `ctx`. The rest of the arguments define the API of the rule.
        Required.
      defaults: a dict with keys matching the rule arguments and values of type
        lucicfg.var(...). These variables can be used to set defaults to use for
        a rule within some exec scope (see lucicfg.var(...) for more details
        about scoping). These vars become the public API of the rule. Callers
        can set them via `rule.defaults.<name>.set(...)`. `impl` callback can
        get them via `ctx.defaults.<name>.get()`. It is up to the rule's author
        to define vars for fields that can have defaults, document them in the
        rule doc, and finally use them from `impl` callback.

    Returns:
      A special callable.
    """
    return __native__.declare_rule(impl, defaults or {})

# Public API.

lucicfg = struct(
    version = _version,
    check_version = _check_version,
    config = _config,
    enable_experiment = _enable_experiment,
    generator = _generator,
    emit = _emit,
    current_module = _current_module,
    var = _var,
    rule = _rule,
)
