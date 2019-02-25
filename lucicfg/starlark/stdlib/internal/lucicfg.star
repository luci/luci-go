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


def _version():
  """Returns a triple with lucicfg version: `(major, minor, revision)`."""
  return __native__.version()


def _config(
      *,
      config_service_host=None,
      config_set=None,
      config_dir=None,
      tracked_files=None,
      fail_on_warnings=None
  ):
  """Sets one or more parameters for the `lucicfg` itself.

  These parameters do not affect semantic meaning of generated configs, but
  influence how they are generated and validated.

  Each parameter has a corresponding command line flag. If the flag is present,
  it overrides the value set via `lucicfg.config` (if any). For example, the
  flag `-config-service-host <value>` overrides whatever was set via
  `lucicfg.config(config_service_host=...)`.

  `lucicfg.config` is allowed to be called multiple times. The most recently set
  value is used in the end, so think of `lucicfg.config(var=...)` just as
  assigning to a variable.

  Args:
    config_service_host: a hostname of a LUCI Config Service to send validation
        requests to. Default is whatever is hardcoded in `lucicfg` binary,
        usually `luci-config.appspot.com`.
    config_set: name of the config set in LUCI Config Service to use for
        validation. Default is `projects/<name>` where `<name>` is taken from
        luci.project(...) rule. If there's no such rule, the default is "",
        meaning the generated config will not be validated via LUCI Config
        Service.
    config_dir: a directory to place generated configs into, relative to the
        directory that contains the entry point \*.star file. `..` is allowed.
        If set via `-config-dir` command line flag, it is relative to the
        current working directory. Will be created if absent. If `-`, the
        configs are just printed to stdout in a format useful for debugging.
        Default is "generated".
    tracked_files: a list of glob patterns that define a subset of files under
        `config_dir` that are considered generated. Each entry is either
        `<glob pattern>` (a "positive" glob) or `!<glob pattern>` (a "negative"
        glob). If only negative globs are given, single positive `*` glob is
        implied as well. A file under `config_dir` (or any of its
        subdirectories) is considered tracked if its base name (not path!)
        matches any of the positive globs and none of the negative globs.
        `tracked_files` can be used to limit what files are actually written to
        disk: if this set is not empty, only files that are in this set will be
        actually written to disk (and all other files are discarded). This is
        beneficial when `lucicfg` is used to generate only a subset of config
        files, e.g. during the  migration from handcrafted to generated configs.
        Knowing the tracked files set is also important when some generated file
        disappears from `lucicfg` output: it must be deleted from the disk as
        well. To do this, `lucicfg` needs to know what files are safe to delete.
        If `tracked_files` is empty (default), `lucicfg` will save all generated
        files and will never delete any file (in this case it is responsibility
        of the caller to make sure no stale output remains).
    fail_on_warnings: if set to True treat validation warnings as errors.
        Default is False (i.e. warnings do to cause the validation to fail). If
        set to True via `lucicfg.config` and you want to override it to False
        via command line flags use `-fail-on-warnings=false`.
  """
  if config_service_host != None:
    __native__.set_meta('config_service_host', config_service_host)
  if config_set != None:
    __native__.set_meta('config_set', config_set)
  if config_dir != None:
    __native__.set_meta('config_dir', config_dir)
  if tracked_files != None:
    __native__.set_meta('tracked_files', tracked_files)
  if fail_on_warnings != None:
    __native__.set_meta('fail_on_warnings', fail_on_warnings)


def _generator(impl):
  """Registers a callback that is called at the end of the config generation
  stage to modify/append/delete generated configs in an arbitrary way.

  The callback accepts single argument `ctx` which is a struct with the
  following fields:

    * **config_set**: a dict `{config file name -> (str | proto)}`.

  The callback is free to modify `ctx.config_set` in whatever way it wants, e.g.
  by adding new values there or mutating/deleting existing ones.

  DocTags:
    Advanced.

  Args:
    impl: a callback `func(ctx) -> None`.
  """
  __native__.add_generator(impl)


def _var(*, default=None, validator=None):
  """Declares a variable.

  A variable is a slot that can hold some frozen value. Initially this slot is
  empty. lucicfg.var(...) returns a struct with methods to manipulate this slot:

    * `set(value)`: sets the variable's value if it's unset, fails otherwise.
    * `get()`: returns the current value, auto-setting it to `default` if it was
      unset.

  Note the auto-setting the value in `get()` means once `get()` is called on an
  unset variable, this variable can't be changed anymore, since it becomes
  initialized and initialized variables are immutable. In effect, all callers of
  `get()` within a scope always observe the exact same value (either an
  explicitly set one, or a default one).

  Any module (loaded or exec'ed) can declare variables via lucicfg.var(...). But
  only modules running through exec(...) can read and write them. Modules being
  loaded via load(...) must not depend on the state of the world while they are
  loading, since they may be loaded at unpredictable moments. Thus an attempt to
  use `get` or `set` from a loading module causes an error.

  Note that functions _exported_ by loaded modules still can do anything they
  want with variables, as long as they are called from an exec-ing module. Only
  code that executes _while the module is loading_ is forbidden to rely on state
  of variables.

  Assignments performed by an exec-ing module are visible only while this module
  and all modules it execs are running. As soon as it finishes, all changes
  made to variable values are "forgotten". Thus variables can be used to
  implicitly propagate information down the exec call stack, but not up (use
  exec's return value for that).

  Generator callbacks registered via lucicfg.generator(...) are forbidden to
  read or write variables, since they execute outside of context of any
  exec(...). Generators must operate exclusively over state stored in the node
  graph. Note that variables still can be used by functions that _build_ the
  graph, they can transfer information from variables into the graph, if
  necessary.

  The most common application for lucicfg.var(...) is to "configure" library
  modules with default values pertaining to some concrete executing script:

    * A library declares variables while it loads and exposes them in its public
      API either directly or via wrapping setter functions.
    * An executing script uses library's public API to set variables' values to
      values relating to what this script does.
    * All calls made to the library from the executing script (or any scripts it
      includes with exec(...)) can access variables' values now.

  This is more magical but less wordy alternative to either passing specific
  default values in every call to library functions, or wrapping all library
  functions with wrappers that supply such defaults. These more explicit
  approaches can become pretty convoluted when there are multiple scripts and
  libraries involved.

  DocTags:
    Advanced.

  Args:
    default: a value to auto-set to the variable in `get()` if it was unset.
    validator: a callback called as `validator(value)` from `set(value)`, must
        return the value to be assigned to the variable (usually just `value`
        itself).

  Returns:
    A struct with two methods: `set(value)` and `get(): value`.
  """
  var_id = __native__.declare_var()

  # The default value (if any) must pass the validation itself.
  if validator and default != None:
    default = validator(default)

  return struct(
      set = lambda v: __native__.set_var(var_id, validator(v) if validator else v),
      get = lambda: __native__.get_var(var_id, default),
  )


# Public API.

lucicfg = struct(
    version = _version,
    config = _config,
    generator = _generator,
    var = _var,
)
