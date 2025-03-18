# LUCI configuration definition language





































[TOC]

## Overview

`lucicfg` is a tool for generating low-level LUCI configuration files based on a
high-level configuration given as a [Starlark] script that uses APIs exposed by
`lucicfg`. In other words, it takes a `*.star` file (or files) as input and
spits out a bunch of `*.cfg` files (such us `cr-buildbucket.cfg` and
`luci-scheduler.cfg`) as outputs. A single entity (such as a [luci.builder(...)](#luci.builder)
definition) in the input is translated into multiple entities (such as
Buildbucket's `builder{...}` and Scheduler's `job{...})` in the output. This ensures
internal consistency of all low-level configs.

Using Starlark allows further reducing duplication and enforcing invariants in
the configs. A common pattern is to use Starlark functions that wrap one or
more basic rules (e.g. [luci.builder(...)](#luci.builder) and [luci.console_view_entry(...)](#luci.console-view-entry)) to
define more "concrete" entities (for example "a CI builder" or "a Try builder").
The rest of the config script then uses such functions to build up the actual
configuration.

### Getting lucicfg

`lucicfg` is distributed as a single self-contained binary as part of
[depot_tools], so if you use them, you already have it. Additionally it is
available in `PATH` on all LUCI builders. The rest of this doc also assumes that
`lucicfg` is in `PATH`.

If you don't use [depot_tools], `lucicfg` can be installed through CIPD. The
package is [infra/tools/luci/lucicfg/${platform}], and the canonical stable
version can be looked up in the depot_tools [CIPD manifest].

Finally, you can always try to build `lucicfg` from the source code. However,
the only officially supported distribution mechanism is CIPD packages.

### Getting started with a simple config

*** note
More examples of using `lucicfg` can be found [here](../examples).
***

Create `main.star` file with the following content:

```python
#!/usr/bin/env lucicfg

luci.project(
    name = "hello-world",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.bucket(name = "my-bucket")

luci.builder(
    name = "my-builder",
    bucket = "my-bucket",
    executable = luci.recipe(
        name = "my-recipe",
        cipd_package = "recipe/bundle/package",
    ),
)
```

Now run `lucicfg generate main.star`. It will create a new directory
`generated` side-by-side with `main.star` file. This directory contains the
`project.cfg` and `cr-buildbucket.cfg` files, generated based on the script
above.

Equivalently, make the script executable (`chmod a+x main.star`) and then just
execute it (`./main.star`). This is the exact same thing as running `generate`
subcommand.

Now make some change in `main.star` (for example, rename the builder), but do
not regenerate the configs yet. Instead run `lucicfg validate main.star`. It
will produce an error, telling you that files on disk (in `generated/*`) are
stale. Regenerate them (`./main.star`), and run the validation again.

If you have never done this before or haven't used any other LUCI tools, you are
now asked to authenticate by running `lucicfg auth-login`. This is because
`lucicfg validate` in addition to checking configs locally also sends them for a
more thorough validation to the LUCI Config service, and this requires you to be
authenticated. Do `lucicfg auth-login` and re-run `lucicfg validate main.star`.
It should succeed now. If it still fails with permissions issues, you are
probably not in `config-validation` group (this should be rare, please contact
luci-eng@google.com if this is happening).

`lucicfg validate` is meant to be used from presubmit tests. If you use
depot_tools' `PRESUBMIT.py`, there's a [canned check] that wraps
`lucicfg validate`.

This is it, your first generated config! It is not very functional yet (e.g.
builders without Swarming dimensions are useless), but a good place to start.
Keep iterating on it, modifying the script, regenerating configs, and examining
the output in `generated` directory. Once you are satisfied with the result,
commit **both** Starlark scripts and generated configs into the repository, and
then configure LUCI Config service to pull configuration from `generated`
directory (how to do it is outside the scope of this doc).

[Starlark]: https://github.com/google/starlark-go
[depot_tools]: https://chromium.googlesource.com/chromium/tools/depot_tools/
[infra/tools/luci/lucicfg/${platform}]: https://chrome-infra-packages.appspot.com/p/infra/tools/luci/lucicfg
[CIPD manifest]: https://chromium.googlesource.com/chromium/tools/depot_tools/+/refs/heads/master/cipd_manifest.txt
[canned check]: https://chromium.googlesource.com/chromium/tools/depot_tools/+/39b0b8e32a4ed0675a38d97799e8a219cc549910/presubmit_canned_checks.py#1437

## Concepts

*** note
Most of information in this section is specific to `lucicfg`, **not** a generic
Starlark interpreter. Also this is **advanced stuff**. Its full understanding is
not required to use `lucicfg` effectively.
***

### Modules and packages {#modules-and-packages}

Each individual Starlark file is called a module. Several modules under the same
root directory form a package. Modules within a single package can refer to each
other (in load(...) and [exec(...)](#exec)) using their relative or absolute (if
start with `//`) paths. The root of the main package is taken to be a directory
that contains the entry point script (usually `main.star`) passed to `lucicfg`,
i.e. `main.star` itself can be referred to as `//main.star`.



Modules can either be "library-like" (executed via load(...) statement) or
"script-like" (executed via [exec(...)](#exec) function). Library-like modules can
load other library-like modules via load(...), but may not call
[exec(...)](#exec). Script-like modules may use both load(...) and [exec(...)](#exec).

Dicts of modules loaded via load(...) are reused, e.g. if two different
scripts load the exact same module, they'll get the exact same symbols as a
result. The loaded code always executes only once. The interpreter *may* load
modules in parallel in the future, libraries must not rely on their loading
order and must not have side effects.

On the other hand, modules executed via [exec(...)](#exec) are guaranteed to be
processed sequentially, and only once. Thus 'exec'-ed scripts essentially form
a tree, traversed exactly once in the depth first order.

### Rules, state representation

All entities manipulated by `lucicfg` are represented by nodes in a directed
acyclic graph. One entity (such as a builder) can internally be represented by
multiple nodes. A function that adds nodes and edges to the graph is called
**a rule** (e.g. [luci.builder(...)](#luci.builder) is a rule).

Each node has a unique hierarchical key, usually constructed from entity's
properties. For example, a builder name and its bucket name are used to
construct a unique key for this builder (roughly `<bucket>/<builder>`). These
keys are used internally by rules when adding edges to the graph.

To refer to entities from public API, one just usually uses strings (e.g.
a builder name to refer to the builder). Rules' implementation usually have
enough context to construct correct node keys from such strings. Sometimes they
need some help, see [Resolving naming ambiguities](#resolving_ambiguities).
Other times entities have no meaningful global names at all (for example,
[luci.console_view_entry(...)](#luci.console-view-entry)). For such cases, one uses a return value of the
corresponding rule: rules return opaque pointer-like objects that can be passed
to other rules as an input in place of a string identifiers. This allows to
"chain" definitions, e.g.

```python
luci.console_view(
    ...
    entries = [
        luci.console_view_entry(...),
        luci.console_view_entry(...),
        ...
    ],
)
```

It is strongly preferred to either use string names to refer to entities **or**
define them inline where they are needed. Please **avoid** storing return values
of rules in variables to refer to them later. Using string names is as powerful
(`lucicfg` verifies referential integrity), and it offers additional advantages
(like referring to entities across file boundaries).

To aid in using inline definitions where makes sense, many rules allow entities
to be defines multiple times as long as all definitions are identical (this is
internally referred to as "idempotent nodes"). It allows following usage style:

```python
def my_recipe(name):
    return luci.recipe(
        name = name,
        cipd_package = 'my/recipe/bundle',
    )

luci.builder(
    name = 'builder 1',
    executable = my_recipe('some-recipe'),
    ...
)

luci.builder(
    name = 'builder 2',
    executable = my_recipe('some-recipe'),
    ...
)
```

Here `some-recipe` is formally defined twice, but both definitions are
identical, so it doesn't cause ambiguities. See the documentation of individual
rules to see whether they allow such redefinitions.

### Execution stages

There are 3 stages of `lucicfg gen` execution:

  1. **Building the state** by executing the given entry `main.star` code and
     all modules it exec's. This builds a graph in memory (via calls to rules),
     and registers a bunch of generator callbacks (via [lucicfg.generator(...)](#lucicfg.generator)) that
     will traverse this graph in the stage 3.
       - Validation of the format of parameters happens during this stage (e.g.
         checking types, ranges, regexps, etc). This is done by rules'
         implementations. A frozen copy of validated parameters is put into
         the added graph nodes to be used from the stage 3.
       - Rules can mutate the graph, but **may not** examine or traverse it.
       - Nodes and edges can be added out of order, e.g. an edge may be added
         before the nodes it connects. Together with the previous constraint, it
         makes most lucicfg statements position independent.
       - The stage ends after reaching the end of the entry `main.star` code. At
         this point we have a (potentially incomplete) graph and a list of
         registered generator callbacks.
  2. **Checking the referential consistency** by verifying all edges of the
     graph actually connect existing nodes. Since we have a lot of information
     about the graph structure, we can emit helpful error messages here, e.g
     `luci.builder("name") refers to undefined luci.bucket("bucket") at <stack
     trace of the corresponding luci.builder(...) definition>`.
       - This stage is performed purely by `lucicfg` core code, not touching
         Starlark at all. It doesn't need to understand the semantics of graph
         nodes, and thus used for all sorts of configs (LUCI configs are just
         one specific application).
       - At the end of the stage we have a consistent graph with no dangling
         edges. It still may be semantically wrong.
  3. **Checking the semantics and generating actual configs** by calling all
     registered generator callbacks sequentially. They can examine and traverse
     the graph in whatever way they want and either emit errors or emit
     generated configs. They **may not** modify the graph at this stage.

Presently all this machinery is mostly hidden from the end user. It will become
available in future versions of `lucicfg` as an API for **extending**
`lucicfg`, e.g. for adding new entity types that have relation to LUCI, or for
repurposing `lucicfg` for generating non-LUCI conifgs.

## Common tasks

### Resolving naming ambiguities {#resolving-ambiguities}

Builder names are scoped to buckets. For example, it is possible to have the
following definition:

```python
# Runs pre-submit tests on Linux.
luci.builder(
    name = 'Linux',
    bucket = 'try',
    ...
)

# Runs post-submit tests on Linux.
luci.builder(
    name = 'Linux',
    bucket = 'ci',
    ...
)
```

Here `Linux` name by itself is ambiguous and can't be used to refer to the
builder. E.g. the following chunk of code will cause an error:

```python
luci.list_view_entry(
    builder = 'Linux',  # but which one?...
    ...
)
```

The fix is to prepend the bucket name:

```python
luci.list_view_entry(
    builder = 'ci/Linux',  # ah, the CI one
    ...
)
```

It is always correct to use "full" name like this. But in practice the vast
majority of real world configs do not have such ambiguities and requiring full
names everywhere is a chore. For that reason `lucicfg` allows to omit the bucket
name if the resulting reference is non-ambiguous. In the example above, if we
remove one of the builders, `builder = 'Linux'` reference becomes valid.


### Referring to builders in other projects {#external-builders}

*** note
**Experimental.** This feature is not yet supported in all contexts. If you want
to refer to an external builder in some rule, check the rule's documentation
to verify it supports such usage. If the documentation doesn't mention external
builders support, then the rule doesn't support it.
***

Some LUCI Services allow one project to refer to resources in another project.
For example, a [luci.console_view(...)](#luci.console-view) can display builders that belong to another
LUCI project, side-by-side with the builders from the project the console
belongs to.

Such external builders can be referred to via their fully qualified name in
the format `<project>:<bucket>/<name>`. Note that `<bucket>` part can't be
omitted.

For example:

```python
luci.console_view_entry(
    builder = 'chromium:ci/Linux Builder',
    ...
)
```

### Defining cron schedules {#schedules-doc}

[luci.builder(...)](#luci.builder) and [luci.gitiles_poller(...)](#luci.gitiles-poller) rules have `schedule` field that
defines how often the builder or poller should run. Schedules are given as
strings. Supported kinds of schedules (illustrated via examples):

  - `* 0 * * * *`: a crontab expression, in a syntax supported by
    https://github.com/gorhill/cronexpr (see its docs for full reference).
    LUCI will attempt to start the job at specified moments in time (based on
    **UTC clock**). Some examples:
      - `0 */3 * * * *` - every 3 hours: at 12:00 AM UTC, 3:00 AM UTC, ...
      - `0 */3 * * *` - the exact same thing (the last field is optional).
      - `0 1/3 * * *` - every 3 hours but starting 1:00 AM UTC.
      - `0 2,10,18 * * *` - at 2 AM UTC, 10 AM UTC, 6 PM UTC.
      - `0 7 * * *` - at 7 AM UTC, once a day.

    If a previous invocation is still running when triggering a new one,
    an overrun is recorded and the new scheduled invocation is skipped. The next
    attempt to start the job happens based on the schedule (not when the
    currently running invocation finishes).

  - `with 10s interval`: run the job in a loop, waiting 10s after finishing
     an invocation before starting a new one. Moments when the job starts aren't
     synchronized with the wall clock at all.

  - `with 1m interval`, `with 1h interval`, `with 1h10m interval`: same format,
    just using minutes and hours instead of seconds.

  - `continuously` is alias for `with 0s interval`, meaning to run the job in
    a loop without any pauses at all.

  - `triggered` schedule indicates that the job is only started via some
    external triggering event (e.g. via LUCI Scheduler API), not periodically.
      - in [luci.builder(...)](#luci.builder) this schedule is useful to make lucicfg setup a
        scheduler job associated with the builder (even if the builder is not
        triggered by anything else in the configs). This exposes the builder in
        LUCI Scheduler API.
      - in [luci.gitiles_poller(...)](#luci.gitiles-poller) this is useful to setup a poller that polls
        only on manual requests, not periodically.


## Formatting and linting Starlark code {#formatting-linting}

lucicfg uses [buildifier] internally to format and lint Starlark code.
Buildifier is primarily intended for Bazel BUILD and \*.bzl files, but it works
with lucicfg's \*.star files reasonably well too.

To format a single Starlark file use `lucicfg fmt path.star`. To format all
\*.star files in a directory (recursively) use `lucicfg fmt <dir>`.

There are two ways to run lint checks:

  1. Per-file or directory using `lucicfg lint <path>`. What set of checks to
     perform can be specified via `-check <set>` argument, where `<set>` is
     a special comma-delimited string that identifies what checks to apply. See
     below for how to construct it.
  2. As part of `lucicfg validate <entry point>.star`. It will check only files
     loaded while executing the entry point script. This is the recommended way.
     The set of checks to apply can be specified via `lint_checks` argument in
     [lucicfg.config(...)](#lucicfg.config), see below for examples. Note that **all checks (including
     formatting checks) are disabled by default for now**. This will change in
     the future.

Checking that files are properly formatted is a special kind of a lint check
called `formatting`.

[buildifier]: https://github.com/bazelbuild/buildtools/tree/master/buildifier


### Specifying a set of linter checks to apply

Both `lucicfg lint -check ...` CLI argument and `lint_checks` in [lucicfg.config(...)](#lucicfg.config)
accept a list of strings that looks like `[<initial set>], +warn1, +warn2,
-warn3, -warn4, ... `, where

  * `<initial set>` can be one of `default`, `none` or `all` and it
    identifies a set of linter checks to use as a base:
    * `default` is a set of checks that are known to work well with lucicfg
      Starlark code. If `<initial set>` is omitted, `default` is used.
    * `none` is an empty set.
    * `all` is all checks known to buildifier. Note that some of them may be
      incompatible with lucicfg Starlark code.
  * `+warn` adds some specific check to the set of checks to apply.
  * `-warn` removes some specific check from the set of checks to apply.

See [buildifier warnings list] for identifiers and meanings of all possible
checks. Note that many of them are specific to Bazel not applicable to lucicfg
Starlark code.

Additionally a check called `formatting` can be used to instruct lucicfg to
verify formatting of Starlark files. It is part of the `default` set. Note that
it is not a built-in buildifier check and thus it's not listed in the buildifier
docs nor can it be disabled via `buildifier: disable=...`.

[buildifier warnings list]: https://github.com/bazelbuild/buildtools/blob/master/WARNINGS.md


### Examples {#linter-config}

To apply all default checks when running `lucicfg validate` use:

```python
lucicfg.config(
    ...
    lint_checks = ["default"],
)
```

This is equivalent to running `lucicfg lint -checks default` or just
`lucicfg lint`.

To check formatting only:

```python
lucicfg.config(
    ...
    lint_checks = ["none", "+formatting"],
)
```

This is equivalent to running `lucicfg lint -checks "none,+formatting"`.

To disable some single default check (e.g. `function-docstring`) globally:

```python
lucicfg.config(
    ...
    lint_checks = ["-function-docstring"],
)
```

This is equivalent to running `lucicfg lint -checks "-function-docstring"`.


### Disabling checks locally

To suppress a specific occurrence of a linter warning add a special comment
`# buildifier: disable=<check-name>` to the expression that causes the warning:

```python
# buildifier: disable=function-docstring
def update_submodules_mirror(
        name,
        short_name,
        source_repo,
        target_repo,
        extra_submodules = None,
        triggered_by = None,
        refs = None):
    properties = {
        "source_repo": source_repo,
        "target_repo": target_repo,
    }
    ...
```

To suppress formatting changes (and thus formatting check) use
`# buildifier: leave-alone`.


## Interfacing with lucicfg internals




### lucicfg.version {#lucicfg.version}

```python
lucicfg.version()
```



Returns a triple with lucicfg version: `(major, minor, revision)`.





### lucicfg.check_version {#lucicfg.check-version}

```python
lucicfg.check_version(min, message = None)
```



Fails if lucicfg version is below the requested minimal one.

Useful when a script depends on some lucicfg feature that may not be
available in earlier versions. [lucicfg.check_version(...)](#lucicfg.check-version) can be used at
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
[lucicfg.check_version(...)](#lucicfg.check-version) in project configs.

#### Arguments {#lucicfg.check-version-args}

* **min**: a string `major.minor.revision` with minimally accepted version. Required.
* **message**: a custom failure message to show.




### lucicfg.config {#lucicfg.config}

```python
lucicfg.config(
    # Optional arguments.
    config_service_host = None,
    config_dir = None,
    tracked_files = None,
    fail_on_warnings = None,
    lint_checks = None,
)
```



Sets one or more parameters for the `lucicfg` itself.

These parameters do not affect semantic meaning of generated configs, but
influence how they are generated and validated.

Each parameter has a corresponding command line flag. If the flag is
present, it overrides the value set via `lucicfg.config` (if any). For
example, the flag `-config-service-host <value>` overrides whatever was set
via `lucicfg.config(config_service_host=...)`.

`lucicfg.config` is allowed to be called multiple times. The most recently
set value is used in the end, so think of `lucicfg.config(var=...)` just as
assigning to a variable.

#### Arguments {#lucicfg.config-args}

* **config_service_host**: a hostname of a LUCI Config Service to send validation requests to. Default is whatever is hardcoded in `lucicfg` binary, usually `config.luci.app`.
* **config_dir**: a directory to place generated configs into, relative to the directory that contains the entry point \*.star file. `..` is allowed. If set via `-config-dir` command line flag, it is relative to the current working directory. Will be created if absent. If `-`, the configs are just printed to stdout in a format useful for debugging. Default is "generated".
* **tracked_files**: a list of glob patterns that define a subset of files under `config_dir` that are considered generated. Each entry is either `<glob pattern>` (a "positive" glob) or `!<glob pattern>` (a "negative" glob). A file under `config_dir` is considered tracked if its slash-separated path matches any of the positive globs and none of the negative globs. If a pattern starts with `**/`, the rest of it is applied to the base name of the file (not the whole path). If only negative globs are given, single positive `**/*` glob is implied as well. `tracked_files` can be used to limit what files are actually emitted: if this set is not empty, only files that are in this set will be actually written to the disk (and all other files are discarded). This is beneficial when `lucicfg` is used to generate only a subset of config files, e.g. during the migration from handcrafted to generated configs. Knowing the tracked files set is also important when some generated file disappears from `lucicfg` output: it must be deleted from the disk as well. To do this, `lucicfg` needs to know what files are safe to delete. If `tracked_files` is empty (default), `lucicfg` will save all generated files and will never delete any file in this case it is responsibility of the caller to make sure no stale output remains).
* **fail_on_warnings**: if set to True treat validation warnings as errors. Default is False (i.e. warnings do not cause the validation to fail). If set to True via `lucicfg.config` and you want to override it to False via command line flags use `-fail-on-warnings=false`.
* **lint_checks**: a list of linter checks to apply in `lucicfg validate`. The first entry defines what group of checks to use as a base and it can be one of `none`, `default` or `all`. The following entries either add checks to the set (`+<name>`) or remove them (`-<name>`). See [Formatting and linting Starlark code](#formatting-linting) for more info. Default is `['none']` for now.




### lucicfg.enable_experiment {#lucicfg.enable-experiment}

```python
lucicfg.enable_experiment(experiment)
```



Enables an experimental feature.

Can be used to experiment with non-default features that may later
change in a non-backwards compatible way or even be removed completely.
Primarily intended for lucicfg developers to test their features before they
are "frozen" to be backward compatible.

Enabling an experiment that doesn't exist logs a warning, but doesn't fail
the execution. Refer to the documentation and the source code for the list
of available experiments.

#### Arguments {#lucicfg.enable-experiment-args}

* **experiment**: a string ID of the experimental feature to enable. Required.




### lucicfg.generator {#lucicfg.generator}

```python
lucicfg.generator(impl = None)
```


*** note
**Advanced function.** It is not used for common use cases.
***


Registers a generator callback.

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

#### Arguments {#lucicfg.generator-args}

* **impl**: a callback `func(ctx) -> None`.




### lucicfg.emit {#lucicfg.emit}

```python
lucicfg.emit(dest, data)
```



Tells lucicfg to write given data to some output file.

In particular useful in conjunction with [io.read_file(...)](#io.read-file) to copy files
into the generated output:

```python
lucicfg.emit(
    dest = "foo.cfg",
    data = io.read_file("//foo.cfg"),
)
```

Note that [lucicfg.emit(...)](#lucicfg.emit) cannot be used to override generated files.
`dest` must refer to a path not generated or emitted by anything else.

#### Arguments {#lucicfg.emit-args}

* **dest**: path to the output file, relative to the `config_dir` (see [lucicfg.config(...)](#lucicfg.config)). Must not start with `../`. Required.
* **data**: either a string or a proto message to write to `dest`. Proto messages are serialized using text protobuf encoding. Required.




### lucicfg.current_module {#lucicfg.current-module}

```python
lucicfg.current_module()
```



Returns the location of a module being currently executed.

This is the module being processed by a current load(...) or [exec(...)](#exec)
statement. It has no relation to the module that holds the top-level stack
frame. For example, if a currently loading module `A` calls a function in
a module `B` and this function calls [lucicfg.current_module(...)](#lucicfg.current-module), the result
would be the module `A`, even though the call goes through code in the
module `B` (i.e. [lucicfg.current_module(...)](#lucicfg.current-module) invocation itself resided in
a function in module `B`).

Fails if called from inside a generator callback. Threads executing such
callbacks are not running any load(...) or [exec(...)](#exec).



#### Returns  {#lucicfg.current-module-returns}

A `struct(package='...', path='...')` with the location of the module.



### lucicfg.var {#lucicfg.var}

```python
lucicfg.var(default = None, validator = None, expose_as = None)
```


*** note
**Advanced function.** It is not used for common use cases.
***


Declares a variable.

A variable is a slot that can hold some frozen value. Initially this slot is
usually empty. [lucicfg.var(...)](#lucicfg.var) returns a struct with methods to manipulate
it:

  * `set(value)`: sets the variable's value if it's unset, fails otherwise.
  * `get()`: returns the current value, auto-setting it to `default` if it
    was unset.

Note the auto-setting the value in `get()` means once `get()` is called on
an unset variable, this variable can't be changed anymore, since it becomes
initialized and initialized variables are immutable. In effect, all callers
of `get()` within a scope always observe the exact same value (either an
explicitly set one, or a default one).

Any module (loaded or exec'ed) can declare variables via [lucicfg.var(...)](#lucicfg.var).
But only modules running through [exec(...)](#exec) can read and write them. Modules
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

Generator callbacks registered via [lucicfg.generator(...)](#lucicfg.generator) are forbidden to
read or write variables, since they execute outside of context of any
[exec(...)](#exec). Generators must operate exclusively over state stored in the node
graph. Note that variables still can be used by functions that _build_ the
graph, they can transfer information from variables into the graph, if
necessary.

The most common application for [lucicfg.var(...)](#lucicfg.var) is to "configure" library
modules with default values pertaining to some concrete executing script:

  * A library declares variables while it loads and exposes them in its
    public API either directly or via wrapping setter functions.
  * An executing script uses library's public API to set variables' values
    to values relating to what this script does.
  * All calls made to the library from the executing script (or any scripts
    it includes with [exec(...)](#exec)) can access variables' values now.

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

#### Arguments {#lucicfg.var-args}

* **default**: a value to auto-set to the variable in `get()` if it was unset.
* **validator**: a callback called as `validator(value)` from `set(value)` and inside [lucicfg.var(...)](#lucicfg.var) declaration itself (to validate `default` or a value passed via CLI flags). Must be a side-effect free idempotent function that returns the value to be assigned to the variable (usually just `value` itself, but conversions are allowed, including type changes).
* **expose_as**: an optional string identifier to make this var settable via CLI flags as `-var <expose_as>=<value>`. If there's no such flag, the variable is auto-initialized to its default value (which must be string or None). Variables declared with `expose_as` are not settable via `set()` at all, they appear as "set" already the moment they are declared. If multiple vars use the same `expose_as` identifier, they will all be initialized to the same value.


#### Returns  {#lucicfg.var-returns}

A struct with two methods: `set(value)` and `get(): value`.



### lucicfg.rule {#lucicfg.rule}

```python
lucicfg.rule(impl, defaults = None)
```


*** note
**Advanced function.** It is not used for common use cases.
***


Declares a new rule.

A rule is a callable that adds nodes and edges to an entity graph. It wraps
the given `impl` callback by passing one additional argument `ctx` to it (as
the first positional argument).

`ctx` is a struct with the following fields:

  * `defaults`: a struct with module-scoped defaults for the rule.

The callback is expected to return a graph.keyset(...) with the set of graph
keys that represent the added node (or nodes). Other rules use such keysets
as inputs.

#### Arguments {#lucicfg.rule-args}

* **impl**: a callback that actually implements the rule. Its first argument should be `ctx`. The rest of the arguments define the API of the rule. Required.
* **defaults**: a dict with keys matching the rule arguments and values of type [lucicfg.var(...)](#lucicfg.var). These variables can be used to set defaults to use for a rule within some exec scope (see [lucicfg.var(...)](#lucicfg.var) for more details about scoping). These vars become the public API of the rule. Callers can set them via `rule.defaults.<name>.set(...)`. `impl` callback can get them via `ctx.defaults.<name>.get()`. It is up to the rule's author to define vars for fields that can have defaults, document them in the rule doc, and finally use them from `impl` callback.


#### Returns  {#lucicfg.rule-returns}

A special callable.





## Working with time

Time module provides a simple API for defining durations in a readable way,
resembling golang's time.Duration.

Durations are represented by integer-like values of [time.duration(...)](#time.duration) type,
which internally hold a number of milliseconds.

Durations can be added and subtracted from each other and multiplied by
integers to get durations. They are also comparable to each other (but not
to integers). Durations can also be divided by each other to get an integer,
e.g. `time.hour / time.second` produces 3600.

The best way to define a duration is to multiply an integer by a corresponding
"unit" constant, for example `10 * time.second`.

Following time constants are exposed:

| Constant           | Value (obviously)         |
|--------------------|---------------------------|
| `time.zero`        | `0 milliseconds`          |
| `time.millisecond` | `1 millisecond`           |
| `time.second`      | `1000 * time.millisecond` |
| `time.minute`      | `60 * time.second`        |
| `time.hour`        | `60 * time.minute`        |
| `time.day`         | `24 * time.hour`          |
| `time.week`        | `7 * time.day`            |


### time.duration {#time.duration}

```python
time.duration(milliseconds)
```



Returns a duration that represents the integer number of milliseconds.

#### Arguments {#time.duration-args}

* **milliseconds**: integer with the requested number of milliseconds. Required.


#### Returns  {#time.duration-returns}

time.duration value.



### time.epoch {#time.epoch}

```python
time.epoch(layout, value, location)
```



Returns epoch seconds for value interpreted as a time per layout in location.

#### Arguments {#time.epoch-args}

* **layout**: a string format showing how the reference time would be interpreted, see golang's time.Parse. Required.
* **value**: a string value to be parsed as a time. Required.
* **location**: a string location, for example 'America/Los_Angeles'. Required.


#### Returns  {#time.epoch-returns}

int epoch seconds for value.



### time.truncate {#time.truncate}

```python
time.truncate(duration, precision)
```



Truncates the precision of the duration to the given value.

For example `time.truncate(time.hour+10*time.minute, time.hour)` is
`time.hour`.

#### Arguments {#time.truncate-args}

* **duration**: a time.duration to truncate. Required.
* **precision**: a time.duration with precision to truncate to. Required.


#### Returns  {#time.truncate-returns}

Truncated time.duration value.



### time.days_of_week {#time.days-of-week}

```python
time.days_of_week(spec)
```



Parses e.g. `Tue,Fri-Sun` into a list of day indexes, e.g. `[2, 5, 6, 7]`.

Monday is 1, Sunday is 7. The returned list is sorted and has no duplicates.
An empty string results in the empty list.

#### Arguments {#time.days-of-week-args}

* **spec**: a case-insensitive string with 3-char abbreviated days of the week. Multiple terms are separated by a comma and optional spaces. Each term is either a day (e.g. `Tue`), or a range (e.g. `Wed-Sun`). Required.


#### Returns  {#time.days-of-week-returns}

A list of 1-based day indexes. Monday is 1.





## Core LUCI rules




### luci.project {#luci.project}

```python
luci.project(
    # Required arguments.
    name,

    # Optional arguments.
    config_dir = None,
    dev = None,
    buildbucket = None,
    logdog = None,
    milo = None,
    notify = None,
    scheduler = None,
    swarming = None,
    change_verifier = None,
    tricium = None,
    acls = None,
    bindings = None,
    enforce_realms_in = None,
    omit_lucicfg_metadata = None,
)
```



Defines a LUCI project.

There should be exactly one such definition in the top-level config file.

This rule also implicitly defines the `@root` realm of the project. It can
be used to setup permissions that apply to all resources in the project. See
[luci.realm(...)](#luci.realm).

#### Arguments {#luci.project-args}

* **name**: full name of the project. Required.
* **config_dir**: a subdirectory of the config output directory (see `config_dir` in [lucicfg.config(...)](#lucicfg.config)) to place generated LUCI configs under. Default is `.`. A custom value is useful when using `lucicfg` to generate LUCI and non-LUCI configs at the same time.
* **dev**: set to True if this project belongs to a development or a staging LUCI deployment. This is rare. Default is False.
* **buildbucket**: appspot hostname of a Buildbucket service to use (if any).
* **logdog**: appspot hostname of a LogDog service to use (if any).
* **milo**: appspot hostname of a Milo service to use (if any).
* **notify**: appspot hostname of a LUCI Notify service to use (if any).
* **scheduler**: appspot hostname of a LUCI Scheduler service to use (if any).
* **swarming**: appspot hostname of a Swarming service to use by default (if any).
* **change_verifier**: appspot hostname of a LUCI Change Verifier (LUCI CV) service to use by default (if any).
* **tricium**: Deprecated. appspot hostname of a Tricium service to use by default (if any).
* **acls**: list of [acl.entry(...)](#acl.entry) objects, will be inherited by all buckets. Being gradually replaced by [luci.binding(...)](#luci.binding) in `bindings`.
* **bindings**: a list of [luci.binding(...)](#luci.binding) to add to the root realm. They will be inherited by all realms in the project. Will eventually replace `acls`.
* **enforce_realms_in**: a list of LUCI service IDs that should enforce realms permissions across all realms. Used only during Realms migration to gradually roll out the enforcement. Can also be enabled realm-by-realm via `enforce_in` in [luci.realm(...)](#luci.realm).
* **omit_lucicfg_metadata**: if True, do not generate `lucicfg {...}` block with lucicfg invocation details in `project.cfg`. This may be useful if you pass frequently changing `-var ...` when generating configs and the resulting generated `lucicfg { vars {...} }` metadata causes frequent merge conflicts. This option is **strongly discouraged** as it makes it impossible to reproducibly regenerate project configs in the LUCI automation (it doesn't know what var values to use). If you use this option, your project may be left out of automatic config migrations. If this happens, you'll need to manually complete the migration on-schedule in order to have your LUCI project continue to function.




### luci.realm {#luci.realm}

```python
luci.realm(
    # Required arguments.
    name,

    # Optional arguments.
    extends = None,
    bindings = None,
    enforce_in = None,
)
```



Defines a realm.

Realm is a named collection of `(<principal>, <permission>)` pairs.

A LUCI resource can point to exactly one realm by referring to its full
name (`<project>:<realm>`). We say that such resource "belongs to the realm"
or "lives in the realm" or is just "in the realm". We also say that such
resource belongs to the project `<project>`. The corresponding
[luci.realm(...)](#luci.realm) definition then describes who can do what to the resource.

The logic of how resources get assigned to realms is a part of the public
API of the service that owns resources. Some services may use a static realm
assignment via project configuration files, others may do it dynamically by
accepting a realm when a resource is created via an RPC.

A realm can "extend" one or more other realms. If a realm `A` extends `B`,
then all permissions defined in `B` are also in `A`. Remembering that a
realm is just a set of `(<principal>, <permission>)` pairs, the "extends"
relation is just a set inclusion.

There are three special realms that a project can have: "@root", "@legacy"
and "@project".

The root realm is implicitly included into all other realms (including
"@legacy"), and it is also used as a fallback when a resource points to a
realm that no longer exists. Without the root realm, such resources become
effectively inaccessible and this may be undesirable. Permissions in the
root realm apply to all realms in the project (current, past and future),
and thus the root realm should contain only administrative-level bindings.
If you are not sure whether you should use the root realm or not, err on
the side of not using it.

The legacy realm is used for existing resources created before the realms
mechanism was introduced. Such resources usually are not associated with any
realm at all. They are implicitly placed into the legacy realm to allow
reusing realms' machinery for them.

Note that the details of how resources are placed in the legacy realm are up
to a particular service implementation. Some services may be able to figure
out an appropriate realm for a legacy resource based on resource's existing
attributes. Some services may not have legacy resources at all. The legacy
realm is not used in these case. Refer to the service documentation.

The project realm should be used as the realm for 'project global' resources,
for example, the project configuration itself, or derivations thereof. Some
LUCI services may use bindings in this realm to allow federation of
administration responsibilities to the project (rather than relying on
exclusively LUCI service administrators).

The primary way of populating the permission set of a realm is via bindings.
Each binding assigns a role to a set of principals (individuals, groups or
LUCI projects). A role is just a set of permissions. A binding grants these
permissions to all principals listed in it.

Binding can be specific either right here:

    luci.realm(
        name = 'try',
        bindings = [
            luci.binding(
                roles = 'role/a',
                groups = ['group-a'],
            ),
            luci.binding(
                roles = 'role/b',
                groups = ['group-b'],
            ),
        ],
    )

Or separately one by one via [luci.binding(...)](#luci.binding) declarations:

    luci.binding(
        realm = 'try',
        roles = 'role/a',
        groups = ['group-a'],
    )
    luci.binding(
        realm = 'try',
        roles = 'role/b',
        groups = ['group-b'],
    )

#### Arguments {#luci.realm-args}

* **name**: name of the realm. Must match `[a-z0-9_\.\-/]{1,400}` or be `@root` or `@legacy`. Required.
* **extends**: a reference or a list of references to realms to inherit permission from. Optional. Default (and implicit) is `@root`.
* **bindings**: a list of [luci.binding(...)](#luci.binding) to add to the realm.
* **enforce_in**: a list of LUCI service IDs that should enforce this realm's permissions. Children realms inherit and extend this list. Used only during Realms migration to gradually roll out the enforcement realm by realm, service by service.




### luci.binding {#luci.binding}

```python
luci.binding(
    # Required arguments.
    roles,

    # Optional arguments.
    realm = None,
    groups = None,
    users = None,
    projects = None,
    conditions = None,
)
```



Binding assigns roles in a realm to individuals, groups or LUCI projects.

A role can either be predefined (if its name starts with `role/`) or custom
(if its name starts with `customRole/`).

Predefined roles are declared in the LUCI deployment configs, see **TODO**
for the up-to-date list of available predefined roles and their meaning.

Custom roles are defined in the project configs via [luci.custom_role(...)](#luci.custom-role).
They can be used if none of the predefined roles represent the desired set
of permissions.

#### Arguments {#luci.binding-args}

* **realm**: a single realm or a list of realms to add the binding to. Can be omitted if the binding is used inline inside some [luci.realm(...)](#luci.realm) declaration.
* **roles**: a single role or a list of roles to assign. Required.
* **groups**: a single group name or a list of groups to assign the role to.
* **users**: a single user email or a list of emails to assign the role to.
* **projects**: a single LUCI project name or a list of project names to assign the role to.
* **conditions**: a list of conditions (ANDed together) that define when this binding is active. Currently only a list of [luci.restrict_attribute(...)](#luci.restrict-attribute) conditions is supported. See [luci.restrict_attribute(...)](#luci.restrict-attribute) for more details. This is an experimental feature.




### luci.restrict_attribute {#luci.restrict-attribute}

```python
luci.restrict_attribute(attribute = None, values = None)
```


*** note
**Experimental.** No backward compatibility guarantees.
***


A condition for [luci.binding(...)](#luci.binding) to restrict allowed attribute values.

When a service checks a permission, it passes to the authorization library
a string-valued dictionary of attributes that describes the context of the
permission check. It contains things like the name of the resource being
accessed, or parameters of the incoming RPC request that triggered the
check.

[luci.restrict_attribute(...)](#luci.restrict-attribute) condition makes the binding active only if
the value of the given attribute is in the given set of allowed values.

A list of available attributes and meaning of their values depends on the
permission being checked and is documented in the corresponding service
documentation.

#### Arguments {#luci.restrict-attribute-args}

* **attribute**: name of the attribute to restrict.
* **values**: a list of strings with allowed values of the attribute.


#### Returns  {#luci.restrict-attribute-returns}

An opaque struct that can be passed to [luci.binding(...)](#luci.binding) as a condition.



### luci.custom_role {#luci.custom-role}

```python
luci.custom_role(name, extends = None, permissions = None)
```



Defines a custom role.

It can be used in [luci.binding(...)](#luci.binding) if predefined roles are too broad or do
not map well to the desired set of permissions.

Custom roles are scoped to the project (i.e. different projects may have
identically named, but semantically different custom roles).

#### Arguments {#luci.custom-role-args}

* **name**: name of the custom role. Must start with `customRole/`. Required.
* **extends**: optional list of roles whose permissions will be included in this role. Each entry can either be a predefined role (if it is a string that starts with `role/`) or another custom role (if it is a string that starts with `customRole/` or a [luci.custom_role(...)](#luci.custom-role) key).
* **permissions**: optional list of permissions to include in the role. Each permission is a symbol that has form `<service>.<subject>.<verb>`, which describes some elementary action (`<verb>`) that can be done to some category of resources (`<subject>`), managed by some particular kind of LUCI service (`<service>`). See **TODO** for the up-to-date list of available permissions and their meaning.




### luci.logdog {#luci.logdog}

```python
luci.logdog(gs_bucket = None, cloud_logging_project = None)
```



Defines configuration of the LogDog service for this project.

Usually required for any non-trivial project.

#### Arguments {#luci.logdog-args}

* **gs_bucket**: base Google Storage archival path, archive logs will be written to this bucket/path.
* **cloud_logging_project**: the name of the Cloud project to export logs.




### luci.bucket {#luci.bucket}

```python
luci.bucket(
    # Required arguments.
    name,

    # Optional arguments.
    acls = None,
    extends = None,
    bindings = None,
    shadows = None,
    constraints = None,
    dynamic = None,
)
```



Defines a bucket: a container for LUCI builds.

This rule also implicitly defines the realm to use for the builds in this
bucket. It can be used to specify permissions that apply to all builds in
this bucket and all resources these builds produce. See [luci.realm(...)](#luci.realm).

#### Arguments {#luci.bucket-args}

* **name**: name of the bucket, e.g. `ci` or `try`. Required.
* **acls**: list of [acl.entry(...)](#acl.entry) objects. Being gradually replaced by [luci.binding(...)](#luci.binding) in `bindings`.
* **extends**: a reference or a list of references to realms to inherit permission from. Note that buckets themselves are realms for this purpose. Optional. Default (and implicit) is `@root`.
* **bindings**: a list of [luci.binding(...)](#luci.binding) to add to the bucket's realm. Will eventually replace `acls`.
* **shadows**: one or a list of bucket names that this bucket shadows. It means that when triggering a led build for the listed buckets (shadowed buckets), Buildbucket will replace the bucket of the led build with this one (shadowing bucket).
* **constraints**: a [luci.bucket_constraints(...)](#luci.bucket-constraints) to add to the bucket.
* **dynamic**: a flag for if the bucket is a dynamic bucket. A dynamic bucket must not have pre-defined builders.




### luci.executable {#luci.executable}

```python
luci.executable(
    # Required arguments.
    name,

    # Optional arguments.
    cipd_package = None,
    cipd_version = None,
    cmd = None,
    wrapper = None,
)
```



Defines an executable.

Builders refer to such executables in their `executable` field, see
[luci.builder(...)](#luci.builder). Multiple builders can execute the same executable
(perhaps passing different properties to it).

Executables must be available as cipd packages.

The cipd version to fetch is usually a lower-cased git ref (like
`refs/heads/main`), or it can be a cipd tag (like `git_revision:abc...`).

A [luci.executable(...)](#luci.executable) with some particular name can be redeclared many
times as long as all fields in all declaration are identical. This is
helpful when [luci.executable(...)](#luci.executable) is used inside a helper function that at
once declares a builder and an executable needed for this builder.

#### Arguments {#luci.executable-args}

* **name**: name of this executable entity, to refer to it from builders. Required.
* **cipd_package**: a cipd package name with the executable. Supports the module-scoped default.
* **cipd_version**: a version of the executable package to fetch, default is `refs/heads/main`. Supports the module-scoped default.
* **cmd**: a list of strings which are the command line to use for this executable. If omitted, either `('recipes',)` or `('luciexe',)` will be used by Buildbucket, according to its global configuration. The special value of `('recipes',)` indicates that this executable should be run under the legacy kitchen runtime. All other values will be executed under the go.chromium.org/luci/luciexe protocol.
* **wrapper**: an optional list of strings which are a command and its arguments to wrap around `cmd`. If set, the builder will run `<wrapper> -- <cmd>`. The 0th argument of the wrapper may be an absolute path. It is up to the owner of the builder to ensure that the wrapper executable is distributed to whatever machines this executable may run on.




### luci.recipe {#luci.recipe}

```python
luci.recipe(
    # Required arguments.
    name,

    # Optional arguments.
    cipd_package = None,
    cipd_version = None,
    recipe = None,
    use_bbagent = None,
    use_python3 = None,
    wrapper = None,
)
```



Defines an executable that runs a particular recipe.

Recipes are python-based DSL for defining what a builder should do, see
[recipes-py](https://chromium.googlesource.com/infra/luci/recipes-py/).

Builders refer to such executable recipes in their `executable` field, see
[luci.builder(...)](#luci.builder). Multiple builders can execute the same recipe (perhaps
passing different properties to it).

Recipes are located inside cipd packages called "recipe bundles". Typically
the cipd package name with the recipe bundle will look like:

    infra/recipe_bundles/chromium.googlesource.com/chromium/tools/build

Recipes bundled from internal repositories are typically under

    infra_internal/recipe_bundles/...

But if you're building your own recipe bundles, they could be located
elsewhere.

The cipd version to fetch is usually a lower-cased git ref (like
`refs/heads/main`), or it can be a cipd tag (like `git_revision:abc...`).

A [luci.recipe(...)](#luci.recipe) with some particular name can be redeclared many times as
long as all fields in all declaration are identical. This is helpful when
[luci.recipe(...)](#luci.recipe) is used inside a helper function that at once declares
a builder and a recipe needed for this builder.

#### Arguments {#luci.recipe-args}

* **name**: name of this recipe entity, to refer to it from builders. If `recipe` is None, also specifies the recipe name within the bundle. Required.
* **cipd_package**: a cipd package name with the recipe bundle. Supports the module-scoped default.
* **cipd_version**: a version of the recipe bundle package to fetch, default is `refs/heads/main`. Supports the module-scoped default.
* **recipe**: name of a recipe inside the recipe bundle if it differs from `name`. Useful if recipe names clash between different recipe bundles. When this happens, `name` can be used as a non-ambiguous alias, and `recipe` can provide the actual recipe name. Defaults to `name`.
* **use_bbagent**: a boolean to override Buildbucket's global configuration. If True, then builders with this recipe will always use bbagent. If False, then builders with this recipe will temporarily stop using bbagent (note that all builders are expected to use bbagent by ~2020Q3). Defaults to unspecified, which will cause Buildbucket to pick according to it's own global configuration. See [this bug](crbug.com/1015181) for the global bbagent rollout. Supports the module-scoped default.
* **use_python3**: a boolean to use python3 to run the recipes. If set, also implies use_bbagent=True. This is equivalent to setting the 'luci.recipes.use_python3' experiment on the builder to 100%. Supports the module-scoped default.
* **wrapper**: an optional list of strings which are a command and its arguments to wrap around recipe execution. If set, the builder will run `<wrapper> -- <luciexe>`. The 0th argument of the wrapper may be an absolute path. It is up to the owner of the builder to ensure that the wrapper executable is distributed to whatever machines this executable may run on.




### luci.builder {#luci.builder}

```python
luci.builder(
    # Required arguments.
    name,
    bucket,
    executable,

    # Optional arguments.
    description_html = None,
    properties = None,
    allowed_property_overrides = None,
    service_account = None,
    caches = None,
    execution_timeout = None,
    grace_period = None,
    heartbeat_timeout = None,
    max_concurrent_builds = None,
    dimensions = None,
    priority = None,
    swarming_host = None,
    swarming_tags = None,
    expiration_timeout = None,
    wait_for_capacity = None,
    retriable = None,
    schedule = None,
    triggering_policy = None,
    build_numbers = None,
    experimental = None,
    experiments = None,
    task_template_canary_percentage = None,
    repo = None,
    resultdb_settings = None,
    test_presentation = None,
    backend = None,
    backend_alt = None,
    shadow_service_account = None,
    shadow_pool = None,
    shadow_properties = None,
    shadow_dimensions = None,
    triggers = None,
    triggered_by = None,
    notifies = None,
    contact_team_email = None,
    custom_metrics = None,
)
```



Defines a generic builder.

It runs some executable (usually a recipe) in some requested environment,
passing it a struct with given properties. It is launched whenever something
triggers it (a poller or some other builder, or maybe some external actor
via Buildbucket or LUCI Scheduler APIs).

The full unique builder name (as expected by Buildbucket RPC interface) is
a pair `(<project>, <bucket>/<name>)`, but within a single project config
this builder can be referred to either via its bucket-scoped name (i.e.
`<bucket>/<name>`) or just via it's name alone (i.e. `<name>`), if this
doesn't introduce ambiguities.

The definition of what can *potentially* trigger what is defined through
`triggers` and `triggered_by` fields. They specify how to prepare ACLs and
other configuration of services that execute builds. If builder **A** is
defined as "triggers builder **B**", it means all services should expect
**A** builds to trigger **B** builds via LUCI Scheduler's EmitTriggers RPC
or via Buildbucket's ScheduleBuild RPC, but the actual triggering is still
the responsibility of **A**'s executable.

There's a caveat though: only Scheduler ACLs are auto-generated by the
config generator when one builder triggers another, because each Scheduler
job has its own ACL and we can precisely configure who's allowed to trigger
this job. Buildbucket ACLs are left unchanged, since they apply to an entire
bucket, and making a large scale change like that (without really knowing
whether Buildbucket API will be used) is dangerous. If the executable
triggers other builds directly through Buildbucket, it is the responsibility
of the config author (you) to correctly specify Buildbucket ACLs, for
example by adding the corresponding service account to the bucket ACLs:

```python
luci.bucket(
    ...
    acls = [
        ...
        acl.entry(acl.BUILDBUCKET_TRIGGERER, <builder service account>),
        ...
    ],
)
```

This is not necessary if the executable uses Scheduler API instead of
Buildbucket.

#### Arguments {#luci.builder-args}

* **name**: name of the builder, will show up in UIs and logs. Required.
* **bucket**: a bucket the builder is in, see [luci.bucket(...)](#luci.bucket) rule. Required.
* **description_html**: description of the builder, will show up in UIs. See https://pkg.go.dev/go.chromium.org/luci/common/data/text/sanitizehtml for the list of allowed HTML elements.
* **executable**: an executable to run, e.g. a [luci.recipe(...)](#luci.recipe) or [luci.executable(...)](#luci.executable). Required.
* **properties**: a dict with string keys and JSON-serializable values, defining properties to pass to the executable. Supports the module-scoped defaults. They are merged (non-recursively) with the explicitly passed properties.
* **allowed_property_overrides**: a list of top-level property keys that can be overridden by users calling the buildbucket ScheduleBuild RPC. If this is set exactly to ['*'], ScheduleBuild is allowed to override any properties. Only property keys which are populated via the `properties` parameter here (or via the module-scoped defaults) are allowed.
* **service_account**: an email of a service account to run the executable under: the executable (and various tools it calls, e.g. gsutil) will be able to make outbound HTTP calls that have an OAuth access token belonging to this service account (provided it is registered with LUCI). Supports the module-scoped default.
* **caches**: a list of [swarming.cache(...)](#swarming.cache) objects describing Swarming named caches that should be present on the bot. See [swarming.cache(...)](#swarming.cache) doc for more details. Supports the module-scoped defaults. They are joined with the explicitly passed caches.
* **execution_timeout**: how long to wait for a running build to finish before forcefully aborting it and marking the build as timed out. If None, defer the decision to Buildbucket service. Supports the module-scoped default.
* **grace_period**: how long to wait after the expiration of `execution_timeout` or after a Cancel event, before the build is forcefully shut down. Your build can use this time as a 'last gasp' to do quick actions like killing child processes, cleaning resources, etc. Supports the module-scoped default.
* **heartbeat_timeout**: How long Buildbucket should wait for a running build to send any updates before forcefully fail it with `INFRA_FAILURE`. If None, Buildbucket won't check the heartbeat timeout. This field only takes effect for builds that don't have Buildbucket managing their underlying backend tasks, namely the ones on TaskBackendLite. E.g. builds running on Swarming don't need to set this.
* **max_concurrent_builds**: maximum number of builds running concurrently for this builder. Builds can be scheduled normally with no limitation. Only the number of builds started is throttled to this value. If set to None, no limit will be enforced to the builder, this is the default flow.
* **dimensions**: a dict with swarming dimensions, indicating requirements for a bot to execute the build. Keys are strings (e.g. `os`), and values are either strings (e.g. `Linux`), [swarming.dimension(...)](#swarming.dimension) objects (for defining expiring dimensions) or lists of thereof. Supports the module-scoped defaults. They are merged (non-recursively) with the explicitly passed dimensions.
* **priority**: int [1-255] or None, indicating swarming task priority, lower is more important. If None, defer the decision to Buildbucket service. Supports the module-scoped default.
* **swarming_host**: appspot hostname of a Swarming service to use for this builder instead of the default specified in [luci.project(...)](#luci.project). Use with great caution. Supports the module-scoped default.
* **swarming_tags**: Deprecated. Used only to enable "vpython:native-python-wrapper" and does not actually propagate to Swarming. A list of tags (`k:v` strings).
* **expiration_timeout**: how long to wait for a build to be picked up by a matching bot (based on `dimensions`) before canceling the build and marking it as expired. If None, defer the decision to Buildbucket service. Supports the module-scoped default.
* **wait_for_capacity**: tell swarming to wait for `expiration_timeout` even if it has never seen a bot whose dimensions are a superset of the requested dimensions. This is useful if this builder has bots whose dimensions are mutated dynamically. Supports the module-scoped default.
* **retriable**: control if the builds on the builder can be retried. Supports the module-scoped default.
* **schedule**: string with a cron schedule that describes when to run this builder. See [Defining cron schedules](#schedules-doc) for the expected format of this field. If None, the builder will not be running periodically.
* **triggering_policy**: [scheduler.policy(...)](#scheduler.policy) struct with a configuration that defines when and how LUCI Scheduler should launch new builds in response to triggering requests from [luci.gitiles_poller(...)](#luci.gitiles-poller) or from EmitTriggers API. Does not apply to builds started directly through Buildbucket. By default, only one concurrent build is allowed and while it runs, triggering requests accumulate in a queue. Once the build finishes, if the queue is not empty, a new build starts right away, "consuming" all pending requests. See [scheduler.policy(...)](#scheduler.policy) doc for more details. Supports the module-scoped default.
* **build_numbers**: if True, generate monotonically increasing contiguous numbers for each build, unique within the builder. If None, defer the decision to Buildbucket service. Supports the module-scoped default.
* **experimental**: if True, by default a new build in this builder will be marked as experimental. This is seen from the executable and it may behave differently (e.g. avoiding any side-effects). If None, defer the decision to Buildbucket service. Supports the module-scoped default.
* **experiments**: a dict that maps experiment name to percentage chance that it will apply to builds generated from this builder. Keys are strings, and values are integers from 0 to 100. This is unrelated to [lucicfg.enable_experiment(...)](#lucicfg.enable-experiment).
* **task_template_canary_percentage**: int [0-100] or None, indicating percentage of builds that should use a canary swarming task template. If None, defer the decision to Buildbucket service. Supports the module-scoped default.
* **repo**: URL of a primary git repository (starting with `https://`) associated with the builder, if known. It is in particular important when using [luci.notifier(...)](#luci.notifier) to let LUCI know what git history it should use to chronologically order builds on this builder. If unknown, builds will be ordered by creation time. If unset, will be taken from the configuration of [luci.gitiles_poller(...)](#luci.gitiles-poller) that trigger this builder if they all poll the same repo.
* **resultdb_settings**: A buildbucket_pb.BuilderConfig.ResultDB, such as one created with [resultdb.settings(...)](#resultdb.settings). A configuration that defines if Buildbucket:ResultDB integration should be enabled for this builder and which results to export to BigQuery.
* **test_presentation**: A [resultdb.test_presentation(...)](#resultdb.test-presentation) struct. A configuration that defines how tests should be rendered in the UI.
* **backend**: the name of the task backend defined via [luci.task_backend(...)](#luci.task-backend). Supports the module-scoped default.
* **backend_alt**: the name of the alternative task backend defined via [luci.task_backend(...)](#luci.task-backend). Supports the module-scoped default.
* **shadow_service_account**: If set, the led builds created for this Builder will instead use this service account. This is useful to allow users to automatically have their testing builds assume a service account which is different than your production service account. When specified, the shadow_service_account will also be included into the shadow bucket's constraints (see [luci.bucket_constraints(...)](#luci.bucket-constraints)). Which also means it will be granted the `role/buildbucket.builderServiceAccount` role in the shadow bucket realm.
* **shadow_pool**: If set, the led builds created for this Builder will instead be set to use this alternate pool instead. This would allow you to grant users the ability to create led builds in the alternate pool without allowing them to create builds in the production pool. When specified, the shadow_pool will also be included into the shadow bucket's constraints (see [luci.bucket_constraints(...)](#luci.bucket-constraints)) and a "pool:<shadow_pool>" dimension will be automatically added to shadow_dimensions.
* **shadow_properties**: If set, the led builds created for this Builder will override the top-level input properties with the same keys.
* **shadow_dimensions**: If set, the led builds created for this Builder will override the dimensions with the same keys. Note: for historical reasons pool can be set individually. If a "pool:<shadow_pool>" dimension is included here, it would have the same effect as setting shadow_pool. shadow_dimensions support dimensions with None values. It's useful for led builds to remove some dimensions the production builds use.
* **triggers**: builders this builder triggers.
* **triggered_by**: builders or pollers this builder is triggered by.
* **notifies**: list of [luci.notifier(...)](#luci.notifier) or [luci.tree_closer(...)](#luci.tree-closer) the builder notifies when it changes its status. This relation can also be defined via `notified_by` field in [luci.notifier(...)](#luci.notifier) or [luci.tree_closer(...)](#luci.tree-closer).
* **contact_team_email**: the owning team's contact email. This team is responsible for fixing any builder health issues (see BuilderConfig.ContactTeamEmail).
* **custom_metrics**: a list of [buildbucket.custom_metric(...)](#buildbucket.custom-metric) objects. Defines the custom metrics this builder should report to.




### luci.gitiles_poller {#luci.gitiles-poller}

```python
luci.gitiles_poller(
    # Required arguments.
    name,
    bucket,
    repo,

    # Optional arguments.
    refs = None,
    path_regexps = None,
    path_regexps_exclude = None,
    schedule = None,
    triggers = None,
)
```



Defines a gitiles poller which can trigger builders on git commits.

It periodically examines the state of watched refs in the git repository. On
each iteration it triggers builders if either:

  * A watched ref's tip has changed since the last iteration (e.g. a new
    commit landed on a ref). Each new detected commit results in a separate
    triggering request, so if for example 10 new commits landed on a ref
    since the last poll, 10 new triggering requests will be submitted to the
    builders triggered by this poller. How they are converted to actual
    builds depends on `triggering_policy` of a builder. For example, some
    builders may want to have one build per commit, others don't care and
    just want to test the latest commit. See [luci.builder(...)](#luci.builder) and
    [scheduler.policy(...)](#scheduler.policy) for more details.

    *** note
    **Caveat**: When a large number of commits are pushed on the ref between
    iterations of the poller, only the most recent 50 commits will result in
    triggering requests. Everything older is silently ignored. This is a
    safeguard against mistaken or deliberate but unusual git push actions,
    which typically don't have the intent of triggering a build for each
    such commit.
    ***

  * A ref belonging to the watched set has just been created. This produces
    a single triggering request for the commit at the ref's tip.
    This also applies right after a configuration change which instructs the
    scheduler to watch a new ref, unless the ref is a tag; only tags that
    appear *after* a change to the poller's refs will be considered new.
    Newly matched but pre-existing tags will not produce triggering
    requests, since there may be many of them and it's rare that a job needs
    to be triggered on all historical tags.

Commits that trigger builders can also optionally be filtered by file paths
they touch. These conditions are specified via `path_regexps` and
`path_regexps_exclude` fields, each is a list of regular expressions against
Unix file paths relative to the repository root. A file is considered
"touched" if it is either added, modified, removed, moved (both old and new
paths are considered "touched"), or its metadata has changed (e.g.
`chmod +x`).

A triggering request is emitted for a commit if only if at least one touched
file is *not* matched by any `path_regexps_exclude` *and* simultaneously
matched by some `path_regexps`, subject to following caveats:

  * `path_regexps = [".+"]` will *not* match commits which modify no files
    (aka empty commits) and as such this situation differs from the default
    case of not specifying any `path_regexps`.
  * As mentioned above, if a ref fast-forwards >=50 commits, only the last
    50 commits are checked. The rest are ignored.

A [luci.gitiles_poller(...)](#luci.gitiles-poller) with some particular name can be redeclared many
times as long as all fields in all declaration are identical. This is
helpful when [luci.gitiles_poller(...)](#luci.gitiles-poller) is used inside a helper function that
at once declares a builder and a poller that triggers this builder.

#### Arguments {#luci.gitiles-poller-args}

* **name**: name of the poller, to refer to it from other rules. Required.
* **bucket**: a bucket the poller is in, see [luci.bucket(...)](#luci.bucket) rule. Required.
* **repo**: URL of a git repository to poll, starting with `https://`. Required.
* **refs**: a list of regular expressions that define the watched set of refs, e.g. `refs/heads/[^/]+` or `refs/branch-heads/\d+\.\d+`. The regular expression should have a literal prefix with at least two slashes present, e.g. `refs/release-\d+/foobar` is *not allowed*, because the literal prefix `refs/release-` contains only one slash. The regexp should not start with `^` or end with `$` as they will be added automatically. Each supplied regexp must match at least one ref in the gitiles output, e.g. specifying `refs/tags/v.+` for a repo that doesn't have tags starting with `v` causes a runtime error. If empty, defaults to `['refs/heads/main']`.
* **path_regexps**: a list of regexps that define a set of files to watch for changes. `^` and `$` are implied and should not be specified manually. See the explanation above for all details.
* **path_regexps_exclude**: a list of regexps that define a set of files to *ignore* when watching for changes. `^` and `$` are implied and should not be specified manually. See the explanation above for all details.
* **schedule**: string with a schedule that describes when to run one iteration of the poller. See [Defining cron schedules](#schedules-doc) for the expected format of this field. Note that it is rare to use custom schedules for pollers. By default, the poller will run each 30 sec.
* **triggers**: builders to trigger whenever the poller detects a new git commit on any ref in the watched ref set.




### luci.milo {#luci.milo}

```python
luci.milo(logo = None, favicon = None, bug_url_template = None)
```



Defines optional configuration of the Milo service for this project.

Milo service is a public user interface for displaying (among other things)
builds, builders, builder lists (see [luci.list_view(...)](#luci.list-view)) and consoles
(see [luci.console_view(...)](#luci.console-view)).

Can optionally be configured with a bug_url_template for filing bugs via
custom bug links on build pages.
The protocol must be `https` and the domain name must be one of the allowed
domains (see [Project.bug_url_template] for details).

The template is interpreted as a [mustache] template and the following
variables are available:
  * {{{ build.builder.project }}}
  * {{{ build.builder.bucket }}}
  * {{{ build.builder.builder }}}
  * {{{ milo_build_url }}}
  * {{{ milo_builder_url }}}

All variables are URL component encoded. Additionally, use `{{{ ... }}}` to
disable HTML escaping. If the template does not satisfy the requirements
above, the link is not displayed.

[Project.bug_url_template]: https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/milo/api/config/project.proto
[mustache]: https://mustache.github.io

#### Arguments {#luci.milo-args}

* **logo**: optional https URL to the project logo (usually \*.png), must be hosted on `storage.googleapis.com`.
* **favicon**: optional https URL to the project favicon (usually \*.ico), must be hosted on `storage.googleapis.com`.
* **bug_url_template**: optional string template for making a custom bug link for filing a bug against a build that displays on the build page.




### luci.list_view {#luci.list-view}

```python
luci.list_view(
    # Required arguments.
    name,

    # Optional arguments.
    title = None,
    favicon = None,
    entries = None,
)
```



A Milo UI view that displays a list of builders.

Builders that belong to this view can be specified either right here:

    luci.list_view(
        name = 'Try builders',
        entries = [
            'win',
            'linux',
            luci.list_view_entry('osx'),
        ],
    )

Or separately one by one via [luci.list_view_entry(...)](#luci.list-view-entry) declarations:

    luci.list_view(name = 'Try builders')
    luci.list_view_entry(
        builder = 'win',
        list_view = 'Try builders',
    )
    luci.list_view_entry(
        builder = 'linux',
        list_view = 'Try builders',
    )

Note that list views support builders defined in other projects. See
[Referring to builders in other projects](#external-builders) for more
details.

#### Arguments {#luci.list-view-args}

* **name**: a name of this view, will show up in URLs. Note that names of [luci.list_view(...)](#luci.list-view) and [luci.console_view(...)](#luci.console-view) are in the same namespace i.e. defining a list view with the same name as some console view (and vice versa) causes an error. Required.
* **title**: a title of this view, will show up in UI. Defaults to `name`.
* **favicon**: optional https URL to the favicon for this view, must be hosted on `storage.googleapis.com`. Defaults to `favicon` in [luci.milo(...)](#luci.milo).
* **entries**: a list of builders or [luci.list_view_entry(...)](#luci.list-view-entry) entities to include into this view.




### luci.list_view_entry {#luci.list-view-entry}

```python
luci.list_view_entry(builder = None, list_view = None)
```



A builder entry in some [luci.list_view(...)](#luci.list-view).

Can be used to declare that a builder belongs to a list view outside of
the list view declaration. In particular useful in functions. For example:

    luci.list_view(name = 'Try builders')

    def try_builder(name, ...):
        luci.builder(name = name, ...)
        luci.list_view_entry(list_view = 'Try builders', builder = name)

Can also be used inline in [luci.list_view(...)](#luci.list-view) declarations, for consistency
with corresponding [luci.console_view_entry(...)](#luci.console-view-entry) usage. `list_view` argument
can be omitted in this case:

    luci.list_view(
        name = 'Try builders',
        entries = [
            luci.list_view_entry(builder = 'Win'),
            ...
        ],
    )

#### Arguments {#luci.list-view-entry-args}

* **builder**: a builder to add, see [luci.builder(...)](#luci.builder). Can also be a reference to a builder defined in another project. See [Referring to builders in other projects](#external-builders) for more details.
* **list_view**: a list view to add the builder to. Can be omitted if `list_view_entry` is used inline inside some [luci.list_view(...)](#luci.list-view) declaration.




### luci.console_view {#luci.console-view}

```python
luci.console_view(
    # Required arguments.
    name,
    repo,

    # Optional arguments.
    title = None,
    refs = None,
    exclude_ref = None,
    header = None,
    include_experimental_builds = None,
    favicon = None,
    default_commit_limit = None,
    default_expand = None,
    entries = None,
)
```



A Milo UI view that displays a table-like console.

In this view columns are builders and rows are git commits on which builders
are triggered.

A console is associated with a single git repository it uses as a source of
commits to display as rows. The watched ref set is defined via `refs` and
optional `exclude_ref` fields. If `refs` are empty, the console defaults to
watching `refs/heads/main`.

`exclude_ref` is useful when watching for commits that landed specifically
on a branch. For example, the config below allows to track commits from all
release branches, but ignore the commits from the main branch, from which
these release branches are branched off:

    luci.console_view(
        ...
        refs = ['refs/branch-heads/\d+\.\d+'],
        exclude_ref = 'refs/heads/main',
        ...
    )

For best results, ensure commits on each watched ref have **committer**
timestamps monotonically non-decreasing. Gerrit will take care of this if
you require each commit to go through Gerrit by prohibiting "git push" on
these refs.

#### Adding builders

Builders that belong to the console can be specified either right here:

    luci.console_view(
        name = 'CI builders',
        ...
        entries = [
            luci.console_view_entry(
                builder = 'Windows Builder',
                short_name = 'win',
                category = 'ci',
            ),
            # Can also pass a dict, this is equivalent to passing
            # luci.console_view_entry(**dict).
            {
                'builder': 'Linux Builder',
                'short_name': 'lnx',
                'category': 'ci',
            },
            ...
        ],
    )

Or separately one by one via [luci.console_view_entry(...)](#luci.console-view-entry) declarations:

    luci.console_view(name = 'CI builders')
    luci.console_view_entry(
        builder = 'Windows Builder',
        console_view = 'CI builders',
        short_name = 'win',
        category = 'ci',
    )

Note that consoles support builders defined in other projects. See
[Referring to builders in other projects](#external-builders) for more
details.

#### Console headers

Consoles can have headers which are collections of links, oncall rotation
information, and console summaries that are displayed at the top of a
console, below the tree status information. Links and oncall information is
always laid out to the left, while console groups are laid out to the right.
Each oncall and links group take up a row.

Header definitions are based on `Header` message in Milo's [project.proto].
There are two way to supply this message via `header` field:

  * Pass an appropriately structured dict. Useful for defining small headers
    inline:

        luci.console_view(
            ...
            header = {
                'links': [
                    {'name': '...', 'links': [...]},
                    ...
                ],
            },
            ...
        )

  * Pass a string. It is treated as a path to a file with serialized
    `Header` message. Depending on its extension, it is loaded ether as
    JSONPB-encoded message (`*.json` and `*.jsonpb` paths), or as
    TextPB-encoded message (everything else):

        luci.console_view(
            ...
            header = '//consoles/main_header.textpb',
            ...
        )

[project.proto]: https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/milo/api/config/project.proto

#### Arguments {#luci.console-view-args}

* **name**: a name of this console, will show up in URLs. Note that names of [luci.console_view(...)](#luci.console-view) and [luci.list_view(...)](#luci.list-view) are in the same namespace i.e. defining a console view with the same name as some list view (and vice versa) causes an error. Required.
* **title**: a title of this console, will show up in UI. Defaults to `name`.
* **repo**: URL of a git repository whose commits are displayed as rows in the console. Must start with `https://`. Required.
* **refs**: a list of regular expressions that define the set of refs to pull commits from when displaying the console, e.g. `refs/heads/[^/]+` or `refs/branch-heads/\d+\.\d+`. The regular expression should have a literal prefix with at least two slashes present, e.g. `refs/release-\d+/foobar` is *not allowed*, because the literal prefix `refs/release-` contains only one slash. The regexp should not start with `^` or end with `$` as they will be added automatically. If empty, defaults to `['refs/heads/main']`.
* **exclude_ref**: a single ref, commits from which are ignored even when they are reachable from refs specified via `refs` and `refs_regexps`. Note that force pushes to this ref are not supported. Milo uses caching assuming set of commits reachable from this ref may only grow, never lose some commits.
* **header**: either a string with a path to the file with the header definition (see [io.read_file(...)](#io.read-file) for the acceptable path format), or a dict with the header definition.
* **include_experimental_builds**: if True, this console will not filter out builds marked as Experimental. By default consoles only show production builds.
* **favicon**: optional https URL to the favicon for this console, must be hosted on `storage.googleapis.com`. Defaults to `favicon` in [luci.milo(...)](#luci.milo).
* **default_commit_limit**: if set, will change the default number of commits to display on a single page.
* **default_expand**: if set, will default the console page to expanded view.
* **entries**: a list of [luci.console_view_entry(...)](#luci.console-view-entry) entities specifying builders to show on the console.




### luci.console_view_entry {#luci.console-view-entry}

```python
luci.console_view_entry(
    # Optional arguments.
    builder = None,
    short_name = None,
    category = None,
    console_view = None,
)
```



A builder entry in some [luci.console_view(...)](#luci.console-view).

Used inline in [luci.console_view(...)](#luci.console-view) declarations to provide `category` and
`short_name` for a builder. `console_view` argument can be omitted in this
case:

    luci.console_view(
        name = 'CI builders',
        ...
        entries = [
            luci.console_view_entry(
                builder = 'Windows Builder',
                short_name = 'win',
                category = 'ci',
            ),
            ...
        ],
    )

Can also be used to declare that a builder belongs to a console outside of
the console declaration. In particular useful in functions. For example:

    luci.console_view(name = 'CI builders')

    def ci_builder(name, ...):
      luci.builder(name = name, ...)
      luci.console_view_entry(console_view = 'CI builders', builder = name)

#### Arguments {#luci.console-view-entry-args}

* **builder**: a builder to add, see [luci.builder(...)](#luci.builder). Can also be a reference to a builder defined in another project. See [Referring to builders in other projects](#external-builders) for more details.
* **short_name**: a shorter name of the builder. The recommendation is to keep this name as short as reasonable, as longer names take up more horizontal space.
* **category**: a string of the form `term1|term2|...` that describes the hierarchy of the builder columns. Neighboring builders with common ancestors will have their column headers merged. In expanded view, each leaf category or builder under a non-leaf category will have it's own column. The recommendation for maximum density is not to mix subcategories and builders for children of each category.
* **console_view**: a console view to add the builder to. Can be omitted if `console_view_entry` is used inline inside some [luci.console_view(...)](#luci.console-view) declaration.




### luci.external_console_view {#luci.external-console-view}

```python
luci.external_console_view(name, source, title = None)
```



Includes a Milo console view from another project.

This console will be listed in the Milo UI on the project page, alongside
the consoles native to this project.

In the following example, we include a console from the 'chromium' project
called 'main', and we give it a local name of 'cr-main' and title of
'Chromium Main Console'.

    luci.external_console_view(
        name = 'cr-main',
        title = 'Chromium Main Console',
        source = 'chromium:main'
    )

#### Arguments {#luci.external-console-view-args}

* **name**: a local name for this console. Will be used for sorting consoles on the project page. Note that the name must not clash with existing consoles or list views in this project. Required.
* **title**: a title for this console, will show up in UI. Defaults to `name`.
* **source**: a string referring to the external console to be included, in the format `project:console_id`. Required.




### luci.notify {#luci.notify}

```python
luci.notify(tree_closing_enabled = None)
```



Defines configuration of the LUCI-Notify service for this project.

#### Arguments {#luci.notify-args}

* **tree_closing_enabled**: if this is set to False, LUCI-Notify won't close trees for this project, just monitor builders and log what actions it would have taken.




### luci.notifier {#luci.notifier}

```python
luci.notifier(
    # Required arguments.
    name,

    # Optional arguments.
    on_occurrence = None,
    on_new_status = None,
    on_failure = None,
    on_new_failure = None,
    on_status_change = None,
    on_success = None,
    failed_step_regexp = None,
    failed_step_regexp_exclude = None,
    notify_emails = None,
    notify_rotation_urls = None,
    notify_blamelist = None,
    blamelist_repos_whitelist = None,
    template = None,
    notified_by = None,
)
```



Defines a notifier that sends notifications on events from builders.

A notifier contains a set of conditions specifying what events are
considered interesting (e.g. a previously green builder has failed), and a
set of recipients to notify when an interesting event happens. The
conditions are specified via `on_*` fields, and recipients are specified
via `notify_*` fields.

The set of builders that are being observed is defined through `notified_by`
field here or `notifies` field in [luci.builder(...)](#luci.builder). Whenever a build
finishes, the builder "notifies" all [luci.notifier(...)](#luci.notifier) objects subscribed
to it, and in turn each notifier filters and forwards this event to
corresponding recipients.

Note that [luci.notifier(...)](#luci.notifier) and [luci.tree_closer(...)](#luci.tree-closer) are both flavors of
a `luci.notifiable` object, i.e. both are something that "can be notified"
when a build finishes. They both are valid targets for `notifies` field in
[luci.builder(...)](#luci.builder). For that reason they share the same namespace, i.e. it is
not allowed to have a [luci.notifier(...)](#luci.notifier) and a [luci.tree_closer(...)](#luci.tree-closer) with
the same name.

#### Arguments {#luci.notifier-args}

* **name**: name of this notifier to reference it from other rules. Required.
* **on_occurrence**: a list specifying which build statuses to notify for. Notifies for every build status specified. Valid values are string literals `SUCCESS`, `FAILURE`, and `INFRA_FAILURE`. Default is None.
* **on_new_status**: a list specifying which new build statuses to notify for. Notifies for each build status specified unless the previous build was the same status. Valid values are string literals `SUCCESS`, `FAILURE`, and `INFRA_FAILURE`. Default is None.
* **on_failure**: Deprecated. Please use `on_new_status` or `on_occurrence` instead. If True, notify on each build failure. Ignores transient (aka "infra") failures. Default is False.
* **on_new_failure**: Deprecated. Please use `on_new_status` or `on_occurrence` instead. If True, notify on a build failure unless the previous build was a failure too. Ignores transient (aka "infra") failures. Default is False.
* **on_status_change**: Deprecated. Please use `on_new_status` or `on_occurrence` instead. If True, notify on each change to a build status (e.g. a green build becoming red and vice versa). Default is False.
* **on_success**: Deprecated. Please use `on_new_status` or `on_occurrence` instead. If True, notify on each build success. Default is False.
* **failed_step_regexp**: an optional regex or list of regexes, which is matched against the names of failed steps. Only build failures containing failed steps matching this regex will cause a notification to be sent. Mutually exclusive with `on_new_status`.
* **failed_step_regexp_exclude**: an optional regex or list of regexes, which has the same function as `failed_step_regexp`, but negated - this regex must *not* match any failed steps for a notification to be sent. Mutually exclusive with `on_new_status`.
* **notify_emails**: an optional list of emails to send notifications to.
* **notify_rotation_urls**: an optional list of URLs from which to fetch rotation members. For each URL, an email will be sent to the currently active member of that rotation. The URL must contain a JSON object, with a field named 'emails' containing a list of email address strings.
* **notify_blamelist**: if True, send notifications to everyone in the computed blamelist for the build. Works only if the builder has a repository associated with it, see `repo` field in [luci.builder(...)](#luci.builder). Default is False.
* **blamelist_repos_whitelist**: an optional list of repository URLs (e.g. `https://host/repo`) to restrict the blamelist calculation to. If empty (default), only the primary repository associated with the builder is considered, see `repo` field in [luci.builder(...)](#luci.builder).
* **template**: a [luci.notifier_template(...)](#luci.notifier-template) to use to format notification emails. If not specified, and a template `default` is defined in the project somewhere, it is used implicitly by the notifier.
* **notified_by**: builders to receive status notifications from. This relation can also be defined via `notifies` field in [luci.builder(...)](#luci.builder).




### luci.tree_closer {#luci.tree-closer}

```python
luci.tree_closer(
    # Required arguments.
    name,
    tree_name,

    # Optional arguments.
    tree_status_host = None,
    failed_step_regexp = None,
    failed_step_regexp_exclude = None,
    template = None,
    notified_by = None,
)
```



Defines a rule for closing or opening a tree via a tree status app.

The set of builders that are being observed is defined through `notified_by`
field here or `notifies` field in [luci.builder(...)](#luci.builder). Whenever a build
finishes, the builder "notifies" all (but usually none or just one)
[luci.tree_closer(...)](#luci.tree-closer) objects subscribed to it, so they can decide whether
to close or open the tree in reaction to the new builder state.

Note that [luci.notifier(...)](#luci.notifier) and [luci.tree_closer(...)](#luci.tree-closer) are both flavors of
a `luci.notifiable` object, i.e. both are something that "can be notified"
when a build finishes. They both are valid targets for `notifies` field in
[luci.builder(...)](#luci.builder). For that reason they share the same namespace, i.e. it is
not allowed to have a [luci.notifier(...)](#luci.notifier) and a [luci.tree_closer(...)](#luci.tree-closer) with
the same name.

#### Arguments {#luci.tree-closer-args}

* **name**: name of this tree closer to reference it from other rules. Required.
* **tree_name**: the identifier of the tree that this rule will open and close. For example, 'chromium'. Tree status affects how CQ lands CLs. See `tree_status_name` in [luci.cq_group(...)](#luci.cq-group). Required.
* **tree_status_host**: **Deprecated**. Please use tree_name instead. A hostname of the project tree status app (if any) that this rule will use to open and close the tree. Tree status affects how CQ lands CLs. See `tree_status_host` in [luci.cq_group(...)](#luci.cq-group).
* **failed_step_regexp**: close the tree only on builds which had a failing step matching this regex, or list of regexes.
* **failed_step_regexp_exclude**: close the tree only on builds which don't have a failing step matching this regex or list of regexes. May be combined with `failed_step_regexp`, in which case it must also have a failed step matching that regular expression.
* **template**: a [luci.notifier_template(...)](#luci.notifier-template) to use to format tree closure notifications. If not specified, and a template `default_tree_status` is defined in the project somewhere, it is used implicitly by the tree closer.
* **notified_by**: builders to receive status notifications from. This relation can also be defined via `notifies` field in [luci.builder(...)](#luci.builder).




### luci.notifier_template {#luci.notifier-template}

```python
luci.notifier_template(name, body)
```



Defines a template to use for notifications from LUCI.

Such template can be referenced by [luci.notifier(...)](#luci.notifier) and
[luci.tree_closer(...)](#luci.tree-closer) rules.

The main template body should have format `<subject>\n\n<body>` where
subject is one line of [text/template] and body is an [html/template]. The
body can either be specified inline right in the starlark script or loaded
from an external file via [io.read_file(...)](#io.read-file).

[text/template]: https://godoc.org/text/template
[html/template]: https://godoc.org/html/template

#### Template input

The input to both templates is a
[TemplateInput](https://godoc.org/go.chromium.org/luci/luci_notify/api/config#TemplateInput)
Go struct derived from
[TemplateInput](https://cs.chromium.org/chromium/infra/go/src/go.chromium.org/luci/luci_notify/api/config/notify.proto?q=TemplateInput)
proto message.

#### Template functions

The following functions are available to templates in addition to the
[standard ones](https://godoc.org/text/template#hdr-Functions).

* `time`: converts a
  [Timestamp](https://pkg.go.dev/google.golang.org/protobuf/types/known/timestamppb#Timestamp)
  to [time.Time](https://godoc.org/time).
  Example: `{{.Build.EndTime | time}}`

#### Template example

```html
A {{.Build.Builder.Builder}} build completed

<a href="https://ci.chromium.org/b/{{.Build.Id}}">Build {{.Build.Number}}</a>
has completed with status {{.Build.Status}}
on `{{.Build.EndTime | time}}`
```

#### Template sharing

A template can "import" subtemplates defined in all other
[luci.notifier_template(...)](#luci.notifier-template). When rendering, *all* templates defined in the
project are merged into one. Example:

```python
# The actual email template which uses subtemplates defined below. In the
# real life it might be better to load such large template from an external
# file using io.read_file.
luci.notifier_template(
    name = 'default',
    body = '\n'.join([
        'A {{.Build.Builder.Builder}} completed',
        '',
        'A <a href="https://ci.chromium.org/b/{{.Build.Id}}">build</a> has completed.',
        '',
        'Steps: {{template "steps" .}}',
        '',
        '{{template "footer"}}',
    ]),
)

# This template renders only steps. It is "executed" by other templates.
luci.notifier_template(
    name = 'steps',
    body = '{{range $step := .Build.Steps}}<li>{{$step.name}}</li>{{end}',
)

# This template defines subtemplates used by other templates.
luci.notifier_template(
    name = 'common',
    body = '{{define "footer"}}Have a nice day!{{end}}',
)
```


#### Email preview

[preview_email](http://godoc.org/go.chromium.org/luci/luci_notify/cmd/preview_email)
command can render a template file to stdout.
```shell
  bb get -json -A 8914184822697034512 | preview_email ./default.template
```

This example uses bb tool, available in
[depot_tools](https://chromium.googlesource.com/chromium/tools/depot_tools/).

Command `preview_email` is available in
[infra Go env](https://chromium.googlesource.com/infra/infra/+/main/go/README.md)
and as a
[CIPD package](https://chrome-infra-packages.appspot.com/p/infra/tools/preview_email).

#### Error handling

If a user-defined template fails to render, a built-in template is used to
generate a very short email with a link to the build and details about the
failure.

#### Arguments {#luci.notifier-template-args}

* **name**: name of this template to reference it from [luci.notifier(...)](#luci.notifier) or [luci.tree_closer(...)](#luci.tree-closer) rules. Must match `^[a-z][a-z0-9\_]*$`. Required.
* **body**: string with the template body. Use [io.read_file(...)](#io.read-file) to load it from an external file, if necessary. Required.




### luci.cq {#luci.cq}

```python
luci.cq(
    # Optional arguments.
    submit_max_burst = None,
    submit_burst_delay = None,
    draining_start_time = None,
    status_host = None,
    honor_gerrit_linked_accounts = None,
)
```



Defines optional configuration of the CQ service for this project.

CQ is a service that monitors Gerrit CLs in a configured set of Gerrit
projects, and launches tryjobs (which run pre-submit tests etc.) whenever a
CL is marked as ready for CQ, and submits the CL if it passes all checks.

**NOTE**: before adding a new [luci.cq(...)](#luci.cq), visit and follow instructions
at http://go/luci/cv/gerrit-pubsub to ensure that pub/sub integration is
enabled for all the Gerrit projects.

This optional rule can be used to set global CQ parameters that apply to all
[luci.cq_group(...)](#luci.cq-group) defined in the project.

#### Arguments {#luci.cq-args}

* **submit_max_burst**: maximum number of successful CQ attempts completed by submitting corresponding Gerrit CL(s) before waiting `submit_burst_delay`. This feature today applies to all attempts processed by CQ, across all [luci.cq_group(...)](#luci.cq-group) instances. Optional, by default there's no limit. If used, requires `submit_burst_delay` to be set too.
* **submit_burst_delay**: how long to wait between bursts of submissions of CQ attempts. Required if `submit_max_burst` is used.
* **draining_start_time**: **Temporarily not supported, see https://crbug.com/1208569. Reach out to LUCI team oncall if you need urgent help.**. If present, the CQ will refrain from processing any CLs, on which CQ was triggered after the specified time. This is an UTC RFC3339 string representing the time, e.g. `2017-12-23T15:47:58Z` and Z is mandatory.
* **status_host**: Optional. Decide whether user has access to the details of runs in this Project in LUCI CV UI. Currently, only the following hosts are accepted: 1) "chromium-cq-status.appspot.com" where everyone can access run details. 2) "internal-cq-status.appspot.com" where only Googlers can access run details. Please don't use the public host if the Project launches internal builders for public repos. It can leak the builder names, which may be confidential.
* **honor_gerrit_linked_accounts**: Optional. Decide whether LUCI CV should consider the primary gerrit accounts and the linked/secondary accounts sharing the same permission. That means if the primary account is allowed to trigger CQ dry run, the secondary account will also be allowed, vice versa.




### luci.cq_group {#luci.cq-group}

```python
luci.cq_group(
    # Required arguments.
    watch,

    # Optional arguments.
    name = None,
    acls = None,
    allow_submit_with_open_deps = None,
    allow_owner_if_submittable = None,
    trust_dry_runner_deps = None,
    allow_non_owner_dry_runner = None,
    tree_status_host = None,
    tree_status_name = None,
    retry_config = None,
    cancel_stale_tryjobs = None,
    verifiers = None,
    additional_modes = None,
    user_limits = None,
    user_limit_default = None,
    post_actions = None,
    tryjob_experiments = None,
)
```



Defines a set of refs to watch and a set of verifier to run.

The CQ will run given verifiers whenever there's a pending approved CL for
a ref in the watched set.

Pro-tip: a command line tool exists to validate a locally generated .cfg
file and verify that it matches arbitrary given CLs as expected.
See https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/cv/#luci-cv-command-line-utils

**NOTE**: if you are configuring a luci.cq_group for a new Gerrit host,
follow instructions at http://go/luci/cv/gerrit-pubsub to ensure that
pub/sub integration is enabled for the Gerrit host.

#### Arguments {#luci.cq-group-args}

* **name**: a human- and machine-readable name this CQ group. Must be unique within this project. This is used in messages posted to users and in monitoring data. Must match regex `^[a-zA-Z][a-zA-Z0-9_-]*$`.
* **watch**: either a single [cq.refset(...)](#cq.refset) or a list of [cq.refset(...)](#cq.refset) (one per repo), defining what set of refs the CQ should monitor for pending CLs. Required.
* **acls**: list of [acl.entry(...)](#acl.entry) objects with ACLs specific for this CQ group. Only `acl.CQ_*` roles are allowed here. By default ACLs are inherited from [luci.project(...)](#luci.project) definition. At least one `acl.CQ_COMMITTER` entry should be provided somewhere (either here or in [luci.project(...)](#luci.project)).
* **allow_submit_with_open_deps**: controls how a CQ full run behaves when the current Gerrit CL has open dependencies (not yet submitted CLs on which *this* CL depends). If set to False (default), the CQ will abort a full run attempt immediately if open dependencies are detected. If set to True, then the CQ will not abort a full run, and upon passing all other verifiers, the CQ will attempt to submit the CL regardless of open dependencies and whether the CQ verified those open dependencies. In turn, if the Gerrit project config allows this, Gerrit will submit all dependent CLs first and then this CL.
* **allow_owner_if_submittable**: allow CL owner to trigger CQ after getting `Code-Review` and other approvals regardless of `acl.CQ_COMMITTER` or `acl.CQ_DRY_RUNNER` roles. Only `cq.ACTION_*` are allowed here. Default is `cq.ACTION_NONE` which grants no additional permissions. CL owner is user owning a CL, i.e. its first patchset uploader, not to be confused with OWNERS files. **WARNING**: using this option is not recommended if you have sticky `Code-Review` label because this allows a malicious developer to upload a good looking patchset at first, get code review approval, and then upload a bad patchset and CQ it right away.
* **trust_dry_runner_deps**: consider CL dependencies that are owned by members of the `acl.CQ_DRY_RUNNER` role as trusted, even if they are not approved. By default, unapproved dependencies are only trusted if they are owned by members of the `acl.CQ_COMMITER` role. This allows CQ dry run on CLs with unapproved dependencies owned by members of `acl.CQ_DRY_RUNNER` role.
* **allow_non_owner_dry_runner**: allow members of the `acl.CQ_DRY_RUNNER` role to trigger DRY_RUN CQ on CLs that are owned by someone else, if all the CL dependencies are trusted.
* **tree_status_host**: **Deprecated**. Please use tree_status_name instead. A hostname of the project tree status app (if any). It is used by the CQ to check the tree status before committing a CL. If the tree is closed, then the CQ will wait until it is reopened.
* **tree_status_name**: the name of the tree that gates CL submission. If the tree is closed, CL will NOT be submitted until the tree is reopened. The tree status UI is at https://ci.chromium.org/ui/tree-status/<tree_status_name>.
* **retry_config**: a new [cq.retry_config(...)](#cq.retry-config) struct or one of `cq.RETRY_*` constants that define how CQ should retry failed builds. See [CQ](#cq-doc) for more info. Default is `cq.RETRY_TRANSIENT_FAILURES`.
* **cancel_stale_tryjobs**: unused anymore, but kept for backward compatibility.
* **verifiers**: a list of [luci.cq_tryjob_verifier(...)](#luci.cq-tryjob-verifier) specifying what checks to run on a pending CL. See [luci.cq_tryjob_verifier(...)](#luci.cq-tryjob-verifier) for all details. As a shortcut, each entry can also either be a dict or a string. A dict is an alias for `luci.cq_tryjob_verifier(**entry)` and a string is an alias for `luci.cq_tryjob_verifier(builder = entry)`.
* **additional_modes**: either a single [cq.run_mode(...)](#cq.run-mode) or a list of [cq.run_mode(...)](#cq.run-mode) defining additional run modes supported by this CQ group apart from standard DRY_RUN and FULL_RUN. If specified, CQ will create the Run with the first mode for which triggering conditions are fulfilled. If there is no such mode, CQ will fallback to standard DRY_RUN or FULL_RUN.
* **user_limits**: a list of [cq.user_limit(...)](#cq.user-limit) or None. They specify per-user limits/quotas for given principals. At the time of a Run start, CV looks up and applies the first matching [cq.user_limit(...)](#cq.user-limit) to the Run, and postpones the start if limits were reached already. If none of the user_limit(s) were applicable, `user_limit_default` will be applied instead. Each [cq.user_limit(...)](#cq.user-limit) must specify at least one user or group.
* **user_limit_default**: [cq.user_limit(...)](#cq.user-limit) or None. If none of limits in `user_limits` are applicable and `user_limit_default` is not specified, the user is granted unlimited runs. `user_limit_default` must not specify users and groups.
* **post_actions**: a list of post actions or None. Please refer to cq.post_action_* for all the available post actions. e.g., [cq.post_action_gerrit_label_votes(...)](#cq.post-action-gerrit-label-votes)
* **tryjob_experiments**: a list of [cq.tryjob_experiment(...)](#cq.tryjob-experiment) or None. The experiments will be enabled when launching Tryjobs if condition is met.




### luci.cq_tryjob_verifier {#luci.cq-tryjob-verifier}

```python
luci.cq_tryjob_verifier(
    # Required arguments.
    builder,

    # Optional arguments.
    cq_group = None,
    result_visibility = None,
    cancel_stale = None,
    includable_only = None,
    disable_reuse = None,
    disable_reuse_footers = None,
    experiment_percentage = None,
    location_filters = None,
    owner_whitelist = None,
    equivalent_builder = None,
    equivalent_builder_percentage = None,
    equivalent_builder_whitelist = None,
    mode_allowlist = None,
)
```



A verifier in a [luci.cq_group(...)](#luci.cq-group) that triggers tryjobs to verify CLs.

When processing a CL, the CQ examines a list of registered verifiers and
launches new corresponding builds (called "tryjobs") if it decides this is
necessary (per the configuration of the verifier and the previous history
of this CL).

The CQ automatically retries failed tryjobs (per configured `retry_config`
in [luci.cq_group(...)](#luci.cq-group)) and only allows CL to land if each builder has
succeeded in the latest retry. If a given tryjob result is too old (>1 day)
it is ignored.

#### Filtering based on files touched by a CL

The CQ can examine a set of files touched by the CL and decide to skip this
verifier. Touching a file means either adding, modifying or removing it.

This is controlled by the `location_filters` field.

location_filters is a list of filters, each of which includes regular
expressions for matching Gerrit host, project, and path. The Gerrit host,
Gerrit project and file path for each file in each CL are matched against
the filters; The last filter that matches all paterns determines whether
the file is considered included (not skipped) or excluded (skipped); if the
last matching LocationFilter has exclude set to true, then the builder is
skipped. If none of the LocationFilters match, then the file is considered
included if the first rule is an exclude rule; else the file is excluded.

The comparison is a full match. The pattern is implicitly anchored with `^`
and `$`, so there is no need add them. The pattern must use [Google
Re2](https://github.com/google/re2) library syntax, [documented
here](https://github.com/google/re2/wiki/Syntax).

This filtering currently cannot be used in any of the following cases:

  * For verifiers in CQ groups with `allow_submit_with_open_deps = True`.

Please talk to CQ owners if these restrictions are limiting you.

##### Examples

Enable the verifier only for all CLs touching any file in `third_party/blink`
directory of the main branch of `chromium/src` repo.

    luci.cq_tryjob_verifier(
        location_filters = [
            cq.location_filter(
                gerrit_host_regexp = 'chromium-review.googlesource.com',
                gerrit_project_regexp = 'chromium/src'
                gerrit_ref_regexp = 'refs/heads/main'
                path_regexp = 'third_party/blink/.+')
        ],
    )

Enable the verifier for CLs that touch files in "foo/", on any host and repo.

    luci.cq_tryjob_verifier(
        location_filters = [
            cq.location_filter(path_regexp = 'foo/.+')
        ],
    )

Disable the verifier for CLs that *only* touches the "all/one.txt" file in
"repo" of "example.com". If the CL touches anything else in the same host
and repo, or touches any file in a different repo and/or host, the verifier
will be enabled.

    luci.cq_tryjob_verifier(
        location_filters = [
            cq.location_filter(
                gerrit_host_regexp = 'example.com',
                gerrit_project_regexp = 'repo',
                path_regexp = 'all/one.txt',
                exclude = True),
        ],
    )

Match a CL which touches at least one file other than `one.txt` inside
`all/` directory of the Gerrit project `repo`:

    luci.cq_tryjob_verifier(
        location_filters = [
            cq.location_filter(
                gerrit_host_regexp = 'example.com',
                gerrit_project_regexp = 'repo',
                path_regexp = 'all/.+'),
            cq.location_filter(
                gerrit_host_regexp = 'example.com',
                gerrit_project_regexp = 'repo',
                path_regexp = 'all/one.txt',
                exclude = True),
        ],
    )

#### Per-CL opt-in only builders

For builders which may be useful only for some CLs, predeclare them using
`includable_only=True` flag. Such builders will be triggered by CQ if and
only if a CL opts in via `CQ-Include-Trybots: <builder>` in its description.

For example, default verifiers may include only fast builders which skip low
level assertions, but for coverage of such assertions one may add slower
"debug" level builders into which CL authors opt-in as needed:

      # triggered & required for all CLs.
      luci.cq_tryjob_verifier(builder="win")
      # triggered & required if only if CL opts in via
      # `CQ-Include-Trybots: project/try/win-debug`.
      luci.cq_tryjob_verifier(builder="win-debug", includable_only=True)

#### Declaring verifiers

`cq_tryjob_verifier` is used inline in [luci.cq_group(...)](#luci.cq-group) declarations to
provide per-builder verifier parameters. `cq_group` argument can be omitted
in this case:

    luci.cq_group(
        name = 'Main CQ',
        ...
        verifiers = [
            luci.cq_tryjob_verifier(
                builder = 'Presubmit',
                disable_reuse = True,
            ),
            ...
        ],
    )


It can also be associated with a [luci.cq_group(...)](#luci.cq-group) outside of
[luci.cq_group(...)](#luci.cq-group) declaration. This is in particular useful in functions.
For example:

    luci.cq_group(name = 'Main CQ')

    def try_builder(name, ...):
        luci.builder(name = name, ...)
        luci.cq_tryjob_verifier(builder = name, cq_group = 'Main CQ')

#### Arguments {#luci.cq-tryjob-verifier-args}

* **builder**: a builder to launch when verifying a CL, see [luci.builder(...)](#luci.builder). Can also be a reference to a builder defined in another project. See [Referring to builders in other projects](#external-builders) for more details. Required.
* **cq_group**: a CQ group to add the verifier to. Can be omitted if `cq_tryjob_verifier` is used inline inside some [luci.cq_group(...)](#luci.cq-group) declaration.
* **result_visibility**: can be used to restrict the visibility of the tryjob results in comments on Gerrit. Valid values are `cq.COMMENT_LEVEL_FULL` and `cq.COMMENT_LEVEL_RESTRICTED` constants. Default is to give full visibility: builder name and full summary markdown are included in the Gerrit comment.
* **cancel_stale**: Controls whether not yet finished builds previously triggered by CQ will be cancelled as soon as a substantially different patchset is uploaded to a CL. Default is True, meaning CQ will cancel. In LUCI Change Verifier (aka CV, successor of CQ), changing this option will only take effect on newly-created Runs once config propagates to CV. Ongoing Runs will retain the old behavior. (TODO(crbug/1127991): refactor this doc after migration. As of 09/2020, CV implementation is WIP)
* **includable_only**: if True, this builder will only be triggered by CQ if it is also specified via `CQ-Include-Trybots:` on CL description. Default is False. See the explanation above for all details. For builders with `experiment_percentage` or `location_filters`, don't specify `includable_only`. Such builders can already be forcefully added via `CQ-Include-Trybots:` in the CL description.
* **disable_reuse**: if True, a fresh build will be required for each CQ attempt. Default is False, meaning the CQ may re-use a successful build triggered before the current CQ attempt started. This option is typically used for verifiers which run presubmit scripts, which are supposed to be quick to run and provide additional OWNERS, lint, etc. checks which are useful to run against the latest revision of the CL's target branch.
* **disable_reuse_footers**: a list of footers for which previous CQ attempts will be not be reused if the footer is added, removed, or has its value changed. Cannot be used together with `disable_reuse = True`, which unconditionally disables reuse.
* **experiment_percentage**: when this field is present, it marks the verifier as experimental. Such verifier is only triggered on a given percentage of the CLs and the outcome does not affect the decision whether a CL can land or not. This is typically used to test new builders and estimate their capacity requirements.
* **location_filters**: a list of [cq.location_filter(...)](#cq.location-filter).
* **owner_whitelist**: a list of groups with accounts of CL owners to enable this builder for. If set, only CLs owned by someone from any one of these groups will be verified by this builder.
* **equivalent_builder**: an optional alternative builder for the CQ to choose instead. If provided, the CQ will choose only one of the equivalent builders as required based purely on the given CL and CL's owner and **regardless** of the possibly already completed try jobs.
* **equivalent_builder_percentage**: a percentage expressing probability of the CQ triggering `equivalent_builder` instead of `builder`. A choice itself is made deterministically based on CL alone, hereby all CQ attempts on all patchsets of a given CL will trigger the same builder, assuming CQ config doesn't change in the mean time. Note that if `equivalent_builder_whitelist` is also specified, the choice over which of the two builders to trigger will be made only for CLs owned by the accounts in the whitelisted group. Defaults to 0, meaning the equivalent builder is never triggered by the CQ, but an existing build can be re-used.
* **equivalent_builder_whitelist**: a group name with accounts to enable the equivalent builder substitution for. If set, only CLs that are owned by someone from this group have a chance to be verified by the equivalent builder. All other CLs are verified via the main builder.
* **mode_allowlist**: a list of modes that CQ will trigger this verifier for. CQ supports `cq.MODE_DRY_RUN` and `cq.MODE_FULL_RUN`, and `cq.MODE_NEW_PATCHSET_RUN` out of the box. Additional Run modes can be defined via `luci.cq_group(additional_modes=...)`.




### luci.bucket_constraints {#luci.bucket-constraints}

```python
luci.bucket_constraints(bucket = None, pools = None, service_accounts = None)
```



Adds constraints to a bucket.

`service_accounts` added as the bucket's constraints will also be granted
`role/buildbucket.builderServiceAccount` role.

Used inline in [luci.bucket(...)](#luci.bucket) declarations to provide `pools` and
`service_accounts` constraints for a bucket. `bucket` argument can be
omitted in this case:

    luci.bucket(
        name = 'try.shadow',
        shadows ='try',
        ...
        constraints = luci.bucket_constraints(
            pools = ['luci.project.shadow'],
            service_accounts = [`shadow@chops-service-account.com`],
        ),
    )

luci.builder function implicitly populates the constraints to the
builder’s bucket. I.e.

    luci.builder(
        'builder',
        bucket = 'ci',
        service_account = 'ci-sa@service-account.com',
    )

adds 'ci-sa@service-account.com' to bucket ci’s constraints.

Can also be used to add constraints to a bucket outside of
the bucket declaration. For example:

    luci.bucket(name = 'ci')
    luci.bucket(name = 'ci.shadow', shadows = 'ci')
    luci.bucket_constraints(bucket = 'ci.shadow', pools = [shadow_pool])

#### Arguments {#luci.bucket-constraints-args}

* **bucket**: name of the bucket to add the constrains.
* **pools**: list of allowed swarming pools to add to the bucket's constraints.
* **service_accounts**: list of allowed service accounts to add to the bucket's constraints.




### luci.buildbucket_notification_topic {#luci.buildbucket-notification-topic}

```python
luci.buildbucket_notification_topic(name, compression = None)
```



Define a buildbucket notification topic.

Buildbucket will publish build notifications (using the luci project scoped
service account) to this topic every time build status changes. For details,
see [BuildbucketCfg.builds_notification_topics]

[BuildbucketCfg.builds_notification_topics]: https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/buildbucket/proto/project_config.proto

#### Arguments {#luci.buildbucket-notification-topic-args}

* **name**: a full topic name. e.g. 'projects/my-cloud-project/topics/my-topic'. Required.
* **compression**: specify a compression method. The default is "ZLIB".




### luci.task_backend {#luci.task-backend}

```python
luci.task_backend(name, target, config = None)
```



Specifies how Buildbucket should integrate with TaskBackend.

#### Arguments {#luci.task-backend-args}

* **name**: A local name of the task backend. Required.
* **target**: URI for this backend, e.g. "swarming://chromium-swarm". Required.
* **config**: A dict with string keys or a proto message to be interpreted as JSON encapsulating configuration for this backend.




### luci.dynamic_builder_template {#luci.dynamic-builder-template}

```python
luci.dynamic_builder_template(
    # Required arguments.
    bucket,

    # Optional arguments.
    executable = None,
    properties = None,
    allowed_property_overrides = None,
    service_account = None,
    caches = None,
    execution_timeout = None,
    grace_period = None,
    heartbeat_timeout = None,
    dimensions = None,
    priority = None,
    expiration_timeout = None,
    retriable = None,
    experiments = None,
    resultdb_settings = None,
    test_presentation = None,
    backend = None,
    backend_alt = None,
    contact_team_email = None,
    custom_metrics = None,
)
```



Defines a dynamic builder template for a dynamic bucket.

#### Arguments {#luci.dynamic-builder-template-args}

* **bucket**: a bucket the builder is in, see [luci.bucket(...)](#luci.bucket) rule. Required.
* **executable**: an executable to run, e.g. a [luci.recipe(...)](#luci.recipe) or [luci.executable(...)](#luci.executable).
* **properties**: a dict with string keys and JSON-serializable values, defining properties to pass to the executable.
* **allowed_property_overrides**: a list of top-level property keys that can be overridden by users calling the buildbucket ScheduleBuild RPC. If this is set exactly to ['*'], ScheduleBuild is allowed to override any properties. Only property keys which are populated via the `properties` parameter here are allowed.
* **service_account**: an email of a service account to run the executable under: the executable (and various tools it calls, e.g. gsutil) will be able to make outbound HTTP calls that have an OAuth access token belonging to this service account (provided it is registered with LUCI).
* **caches**: a list of [swarming.cache(...)](#swarming.cache) objects describing Swarming named caches that should be present on the bot. See [swarming.cache(...)](#swarming.cache) doc for more details.
* **execution_timeout**: how long to wait for a running build to finish before forcefully aborting it and marking the build as timed out. If None, defer the decision to Buildbucket service.
* **grace_period**: how long to wait after the expiration of `execution_timeout` or after a Cancel event, before the build is forcefully shut down. Your build can use this time as a 'last gasp' to do quick actions like killing child processes, cleaning resources, etc.
* **heartbeat_timeout**: How long Buildbucket should wait for a running build to send any updates before forcefully fail it with `INFRA_FAILURE`. If None, Buildbucket won't check the heartbeat timeout. This field only takes effect for builds that don't have Buildbucket managing their underlying backend tasks, namely the ones on TaskBackendLite. E.g. builds running on Swarming don't need to set this.
* **dimensions**: a dict with swarming dimensions, indicating requirements for a bot to execute the build. Keys are strings (e.g. `os`), and values are either strings (e.g. `Linux`), [swarming.dimension(...)](#swarming.dimension) objects (for defining expiring dimensions) or lists of thereof.
* **priority**: int [1-255] or None, indicating swarming task priority, lower is more important. If None, defer the decision to Buildbucket service.
* **expiration_timeout**: how long to wait for a build to be picked up by a matching bot (based on `dimensions`) before canceling the build and marking it as expired. If None, defer the decision to Buildbucket service.
* **retriable**: control if the builds on the builder can be retried.
* **experiments**: a dict that maps experiment name to percentage chance that it will apply to builds generated from this builder. Keys are strings, and values are integers from 0 to 100. This is unrelated to [lucicfg.enable_experiment(...)](#lucicfg.enable-experiment).
* **resultdb_settings**: A buildbucket_pb.BuilderConfig.ResultDB, such as one created with [resultdb.settings(...)](#resultdb.settings). A configuration that defines if Buildbucket:ResultDB integration should be enabled for this builder and which results to export to BigQuery.
* **test_presentation**: A [resultdb.test_presentation(...)](#resultdb.test-presentation) struct. A configuration that defines how tests should be rendered in the UI.
* **backend**: the name of the task backend defined via [luci.task_backend(...)](#luci.task-backend).
* **backend_alt**: the name of the alternative task backend defined via [luci.task_backend(...)](#luci.task-backend).
* **contact_team_email**: the owning team's contact email. This team is responsible for fixing any builder health issues (see BuilderConfig.ContactTeamEmail).
* **custom_metrics**: a list of buildbucket_pb.CustomMetricDefinition() protos, returned by [buildbucket.custom_metric(...)](#buildbucket.custom-metric). Defines the custom metrics the builders created by this template should report to.






## ACLs

### Permissions and roles in LUCI Realms {#realm-roles-doc}

See [permissions.cfg](https://source.corp.google.com/h/chromium/infra/infra_superproject/+/main:data/config/configs/chrome-infra-auth/permissions.cfg)
(sorry, internal only) for permissions and roles in LUCI Realms.


### Roles {#roles-doc}

Below is the table with role constants that can be passed as `roles` in
[acl.entry(...)](#acl.entry).

Due to some inconsistencies in how LUCI service are currently implemented, some
roles can be assigned only in [luci.project(...)](#luci.project) rule, but some also in individual
[luci.bucket(...)](#luci.bucket) or [luci.cq_group(...)](#luci.cq-group) rules.

Similarly some roles can be assigned to individual users, other only to groups.

| Role  | Scope | Principals | Allows |
|-------|-------|------------|--------|
| acl.PROJECT_CONFIGS_READER |project only |groups, users |Reading contents of project configs through LUCI Config API/UI. |
| acl.LOGDOG_READER |project only |groups |Reading logs under project's logdog prefix. |
| acl.LOGDOG_WRITER |project only |groups |Writing logs under project's logdog prefix. |
| acl.BUILDBUCKET_READER |project, bucket |groups, users |Fetching info about a build, searching for builds in a bucket. |
| acl.BUILDBUCKET_TRIGGERER |project, bucket |groups, users |Same as `BUILDBUCKET_READER` + scheduling and canceling builds. |
| acl.BUILDBUCKET_OWNER |project, bucket |groups, users |Full access to the bucket (should be used rarely). |
| acl.SCHEDULER_READER |project, bucket |groups, users |Viewing Scheduler jobs, invocations and their debug logs. |
| acl.SCHEDULER_TRIGGERER |project, bucket |groups, users |Same as `SCHEDULER_READER` + ability to trigger jobs. |
| acl.SCHEDULER_OWNER |project, bucket |groups, users |Full access to Scheduler jobs, including ability to abort them. |
| acl.CQ_COMMITTER |project, cq_group |groups |Committing approved CLs via CQ. |
| acl.CQ_DRY_RUNNER |project, cq_group |groups |Executing presubmit tests for CLs via CQ. |
| acl.CQ_NEW_PATCHSET_RUN_TRIGGERER |project, cq_group |groups |Having LUCI CV run tryjobs (e.g. static analyzers) on new patchset upload. |





### acl.entry {#acl.entry}

```python
acl.entry(
    # Required arguments.
    roles,

    # Optional arguments.
    groups = None,
    users = None,
    projects = None,
)
```



Returns a new ACL binding.

It assign the given role (or roles) to given individuals, groups or LUCI
projects.

Lists of acl.entry structs are passed to `acls` fields of [luci.project(...)](#luci.project)
and [luci.bucket(...)](#luci.bucket) rules.

An empty ACL binding is allowed. It is ignored everywhere. Useful for things
like:

```python
luci.project(
    acls = [
        acl.entry(acl.PROJECT_CONFIGS_READER, groups = [
            # TODO: members will be added later
        ])
    ]
)
```

#### Arguments {#acl.entry-args}

* **roles**: a single role or a list of roles to assign. Required.
* **groups**: a single group name or a list of groups to assign the role to.
* **users**: a single user email or a list of emails to assign the role to.
* **projects**: a single LUCI project name or a list of project names to assign the role to.


#### Returns  {#acl.entry-returns}

acl.entry object, should be treated as opaque.





## ResultDB




### resultdb.settings {#resultdb.settings}

```python
resultdb.settings(enable = None, bq_exports = None, history_options = None)
```



Specifies how Buildbucket should integrate with ResultDB.

#### Arguments {#resultdb.settings-args}

* **enable**: boolean, whether to enable ResultDB:Buildbucket integration.
* **bq_exports**: list of resultdb_pb.BigQueryExport() protos, configurations for exporting specific subsets of test results to a designated BigQuery table, use [resultdb.export_test_results(...)](#resultdb.export-test-results) to create these.
* **history_options**: Configuration for indexing test results from this builder's builds for history queries, use [resultdb.history_options(...)](#resultdb.history-options) to create this value.


#### Returns  {#resultdb.settings-returns}

A populated buildbucket_pb.BuilderConfig.ResultDB() proto.



### resultdb.export_test_results {#resultdb.export-test-results}

```python
resultdb.export_test_results(bq_table = None, predicate = None)
```



Defines a mapping between a test results and a BigQuery table for them.

#### Arguments {#resultdb.export-test-results-args}

* **bq_table**: Tuple of `(project, dataset, table)`; OR a string of the form `<project>.<dataset>.<table>` where the parts represent the BigQuery-enabled gcp project, dataset and table to export results.
* **predicate**: A predicate_pb.TestResultPredicate() proto. If given, specifies the subset of test results to export to the above table, instead of all. Use [resultdb.test_result_predicate(...)](#resultdb.test-result-predicate) to generate this, if needed.


#### Returns  {#resultdb.export-test-results-returns}

A populated resultdb_pb.BigQueryExport() proto.



### resultdb.test_result_predicate {#resultdb.test-result-predicate}

```python
resultdb.test_result_predicate(
    # Optional arguments.
    test_id_regexp = None,
    variant = None,
    variant_contains = None,
    unexpected_only = None,
)
```



Represents a predicate of test results.

#### Arguments {#resultdb.test-result-predicate-args}

* **test_id_regexp**: string, regular expression that a test result must fully match to be considered covered by this definition.
* **variant**: string dict, defines the test variant to match. E.g. `{"test_suite": "not_site_per_process_webkit_layout_tests"}`
* **variant_contains**: bool, if true the variant parameter above will cause a match if it's a subset of the test's variant, otherwise it will only match if it's exactly equal.
* **unexpected_only**: bool, if true only export test results of test variants that had unexpected results.


#### Returns  {#resultdb.test-result-predicate-returns}

A populated predicate_pb.TestResultPredicate() proto.



### resultdb.validate_settings {#resultdb.validate-settings}

```python
resultdb.validate_settings(attr, settings = None)
```



Validates the type of a ResultDB settings proto.

#### Arguments {#resultdb.validate-settings-args}

* **attr**: field name with settings, for error messages. Required.
* **settings**: A proto such as the one returned by [resultdb.settings(...)](#resultdb.settings).


#### Returns  {#resultdb.validate-settings-returns}

A validated proto, if it's the correct type.



### resultdb.history_options {#resultdb.history-options}

```python
resultdb.history_options(by_timestamp = None)
```



Defines a history indexing configuration.

#### Arguments {#resultdb.history-options-args}

* **by_timestamp**: bool, indicates whether the build's test results will be indexed by their creation timestamp for the purposes of retrieving the history of a given set of tests/variants.


#### Returns  {#resultdb.history-options-returns}

A populated resultdb_pb.HistoryOptions() proto.



### resultdb.export_text_artifacts {#resultdb.export-text-artifacts}

```python
resultdb.export_text_artifacts(bq_table = None, predicate = None)
```



Defines a mapping between text artifacts and a BigQuery table for them.

#### Arguments {#resultdb.export-text-artifacts-args}

* **bq_table**: string of the form `<project>.<dataset>.<table>` where the parts respresent the BigQuery-enabled gcp project, dataset and table to export results.
* **predicate**: A predicate_pb.ArtifactPredicate() proto. If given, specifies the subset of text artifacts to export to the above table, instead of all. Use [resultdb.artifact_predicate(...)](#resultdb.artifact-predicate) to generate this, if needed.


#### Returns  {#resultdb.export-text-artifacts-returns}

A populated resultdb_pb.BigQueryExport() proto.



### resultdb.artifact_predicate {#resultdb.artifact-predicate}

```python
resultdb.artifact_predicate(
    # Optional arguments.
    test_result_predicate = None,
    included_invocations = None,
    test_results = None,
    content_type_regexp = None,
    artifact_id_regexp = None,
)
```



Represents a predicate of text artifacts.

#### Arguments {#resultdb.artifact-predicate-args}

* **test_result_predicate**: predicate_pb.TestResultPredicate(), a predicate of test results.
* **included_invocations**: bool, if true, invocation level artifacts are included.
* **test_results**: bool, if true, test result level artifacts are included.
* **content_type_regexp**: string, an artifact must have a content type matching this regular expression entirely, i.e. the expression is implicitly wrapped with ^ and $.
* **artifact_id_regexp**: string, an artifact must have an ID matching this regular expression entirely, i.e. the expression is implicitly wrapped with ^ and $.


#### Returns  {#resultdb.artifact-predicate-returns}

A populated predicate_pb.ArtifactPredicate() proto.



### resultdb.test_presentation {#resultdb.test-presentation}

```python
resultdb.test_presentation(column_keys = None, grouping_keys = None)
```



Specifies how test should be rendered.

#### Arguments {#resultdb.test-presentation-args}

* **column_keys**: list of string keys that will be rendered as 'columns'. status is always the first column and name is always the last column (you don't need to specify them). A key must be one of the following:   1. 'v.{variant_key}': variant.def[variant_key] of the test variant     (e.g. v.gpu). If None, defaults to [].
* **grouping_keys**: list of string keys that will be used for grouping tests. A key must be one of the following:   1. 'status': status of the test variant.   2. 'name': name of the test variant.   3. 'v.{variant_key}': variant.def[variant_key] of the test variant     (e.g. v.gpu). If None, defaults to ['status']. Caveat: test variants with only expected results are not affected by   this setting and are always in their own group.


#### Returns  {#resultdb.test-presentation-returns}

test_presentation.config struct with fields `column_keys` and
`grouping_keys`.



### resultdb.validate_test_presentation {#resultdb.validate-test-presentation}

```python
resultdb.validate_test_presentation(attr, config = None, required = None)
```



Validates a test presentation config.

#### Arguments {#resultdb.validate-test-presentation-args}

* **attr**: field name with caches, for error messages. Required.
* **config**: a test_presentation.config to validate.
* **required**: if False, allow 'config' to be None, return None in this case.


#### Returns  {#resultdb.validate-test-presentation-returns}

A validated test_presentation.config.





## Swarming




### swarming.cache {#swarming.cache}

```python
swarming.cache(path, name = None, wait_for_warm_cache = None)
```



Represents a request for the bot to mount a named cache to a path.

Each bot has a LRU of named caches: think of them as local named directories
in some protected place that survive between builds.

A build can request one or more such caches to be mounted (in read/write
mode) at the requested path relative to some known root. In recipes-based
builds, the path is relative to `api.paths['cache']` dir.

If it's the first time a cache is mounted on this particular bot, it will
appear as an empty directory. Otherwise it will contain whatever was left
there by the previous build that mounted exact same named cache on this bot,
even if that build is completely irrelevant to the current build and just
happened to use the same named cache (sometimes this is useful to share
state between different builders).

At the end of the build the cache directory is unmounted. If at that time
the bot is running out of space, caches (in their entirety, the named cache
directory and all files inside) are evicted in LRU manner until there's
enough free disk space left. Renaming a cache is equivalent to clearing it
from the builder perspective. The files will still be there, but eventually
will be purged by GC.

Additionally, Buildbucket always implicitly requests to mount a special
builder cache to 'builder' path:

    swarming.cache('builder', name=some_hash('<project>/<bucket>/<builder>'))

This means that any LUCI builder has a "personal disk space" on the bot.
Builder cache is often a good start before customizing caching. In recipes,
it is available at `api.path['cache'].join('builder')`.

In order to share the builder cache directory among multiple builders, some
explicitly named cache can be mounted to `builder` path on these builders.
Buildbucket will not try to override it with its auto-generated builder
cache.

For example, if builders **A** and **B** both declare they use named cache
`swarming.cache('builder', name='my_shared_cache')`, and an **A** build ran
on a bot and left some files in the builder cache, then when a **B** build
runs on the same bot, the same files will be available in its builder cache.

If the pool of swarming bots is shared among multiple LUCI projects and
projects mount same named cache, the cache will be shared across projects.
To avoid affecting and being affected by other projects, prefix the cache
name with something project-specific, e.g. `v8-`.

#### Arguments {#swarming.cache-args}

* **path**: path where the cache should be mounted to, relative to some known root (in recipes this root is `api.path['cache']`). Must use POSIX format (forward slashes). In most cases, it does not need slashes at all. Must be unique in the given builder definition (cannot mount multiple caches to the same path). Required.
* **name**: identifier of the cache to mount to the path. Default is same value as `path` itself. Must be unique in the given builder definition (cannot mount the same cache to multiple paths).
* **wait_for_warm_cache**: how long to wait (with minutes precision) for a bot that has this named cache already to become available and pick up the build, before giving up and starting looking for any matching bot (regardless whether it has the cache or not). If there are no bots with this cache at all, the build will skip waiting and will immediately fallback to any matching bot. By default (if unset or zero), there'll be no attempt to find a bot with this cache already warm: the build may or may not end up on a warm bot, there's no guarantee one way or another.


#### Returns  {#swarming.cache-returns}

swarming.cache struct with fields `path`, `name` and `wait_for_warm_cache`.



### swarming.dimension {#swarming.dimension}

```python
swarming.dimension(value, expiration = None)
```



A value of some Swarming dimension, annotated with its expiration time.

Intended to be used as a value in `dimensions` dict of [luci.builder(...)](#luci.builder)
when using dimensions that expire:

```python
luci.builder(
    ...
    dimensions = {
        ...
        'device': swarming.dimension('preferred', expiration=5*time.minute),
        ...
    },
    ...
)
```

#### Arguments {#swarming.dimension-args}

* **value**: string value of the dimension. Required.
* **expiration**: how long to wait (with minutes precision) for a bot with this dimension to become available and pick up the build, or None to wait until the overall build expiration timeout.


#### Returns  {#swarming.dimension-returns}

swarming.dimension struct with fields `value` and `expiration`.



### swarming.validate_caches {#swarming.validate-caches}

```python
swarming.validate_caches(attr, caches)
```


*** note
**Advanced function.** It is not used for common use cases.
***


Validates a list of caches.

Ensures each entry is swarming.cache struct, and no two entries use same
name or path.

#### Arguments {#swarming.validate-caches-args}

* **attr**: field name with caches, for error messages. Required.
* **caches**: a list of [swarming.cache(...)](#swarming.cache) entries to validate. Required.


#### Returns  {#swarming.validate-caches-returns}

Validates list of caches (may be an empty list, never None).



### swarming.validate_dimensions {#swarming.validate-dimensions}

```python
swarming.validate_dimensions(attr, dimensions, allow_none = None)
```


*** note
**Advanced function.** It is not used for common use cases.
***


Validates and normalizes a dict with dimensions.

The dict should have string keys and values are swarming.dimension, a string
or a list of thereof (for repeated dimensions).

#### Arguments {#swarming.validate-dimensions-args}

* **attr**: field name with dimensions, for error messages. Required.
* **dimensions**: a dict `{string: string|swarming.dimension}`. Required.
* **allow_none**: if True, allow None values (indicates absence of the dimension).


#### Returns  {#swarming.validate-dimensions-returns}

Validated and normalized dict in form `{string: [swarming.dimension]}`.



### swarming.validate_tags {#swarming.validate-tags}

```python
swarming.validate_tags(attr, tags)
```


*** note
**Advanced function.** It is not used for common use cases.
***


Validates a list of `k:v` pairs with Swarming tags.

#### Arguments {#swarming.validate-tags-args}

* **attr**: field name with tags, for error messages. Required.
* **tags**: a list of tags to validate. Required.


#### Returns  {#swarming.validate-tags-returns}

Validated list of tags in same order, with duplicates removed.





## Scheduler




### scheduler.policy {#scheduler.policy}

```python
scheduler.policy(
    # Required arguments.
    kind,

    # Optional arguments.
    max_concurrent_invocations = None,
    max_batch_size = None,
    log_base = None,
    pending_timeout = None,
)
```



Policy for how LUCI Scheduler should handle incoming triggering requests.

This policy defines when and how LUCI Scheduler should launch new builds in
response to triggering requests from [luci.gitiles_poller(...)](#luci.gitiles-poller) or from
EmitTriggers RPC call.

The following batching strategies are supported:

  * `scheduler.GREEDY_BATCHING_KIND`: use a greedy batching function that
    takes all pending triggering requests (up to `max_batch_size` limit) and
    collapses them into one new build. It doesn't wait for a full batch, nor
    tries to batch evenly.
  * `scheduler.LOGARITHMIC_BATCHING_KIND`: use a logarithmic batching
    function that takes `floor(log(base,N))` pending triggers (at least 1
    and up to `max_batch_size` limit) and collapses them into one new build,
    where N is the total number of pending triggers. The base of the
    logarithm is defined by `log_base`.
  * `scheduler.NEWEST_FIRST`: use a function that prioritizes the most recent
     pending triggering requests. Triggers stay pending until they either
    become the most recent pending triggering request or expire. The timeout
    for pending triggers is specified by `pending_timeout`.

#### Arguments {#scheduler.policy-args}

* **kind**: one of `*_BATCHING_KIND` values above. Required.
* **max_concurrent_invocations**: limit on a number of builds running at the same time. If the number of currently running builds launched through LUCI Scheduler is more than or equal to this setting, LUCI Scheduler will keep queuing up triggering requests, waiting for some running build to finish before starting a new one. Default is 1.
* **max_batch_size**: limit on how many pending triggering requests to "collapse" into a new single build. For example, setting this to 1 will make each triggering request result in a separate build. When multiple triggering request are collapsed into a single build, properties of the most recent triggering request are used to derive properties for the build. For example, when triggering requests come from a [luci.gitiles_poller(...)](#luci.gitiles-poller), only a git revision from the latest triggering request (i.e. the latest commit) will end up in the build properties. This value is ignored by NEWEST_FIRST, since batching isn't well-defined in that policy kind. Default is 1000 (effectively unlimited).
* **log_base**: base of the logarithm operation during logarithmic batching. For example, setting this to 2, will cause 3 out of 8 pending triggering requests to be combined into a single build. Required when using `LOGARITHMIC_BATCHING_KIND`, ignored otherwise. Must be larger or equal to 1.0001 for numerical stability reasons.
* **pending_timeout**: how long until a pending trigger is discarded. For example, setting this to 1 day will cause triggers that stay pending for at least 1 day to be removed from consideration. This value is ignored by policy kinds other than NEWEST_FIRST, which can starve old triggers and cause the pending triggers list to grow without bound. Default is 7 days.


#### Returns  {#scheduler.policy-returns}

An opaque triggering policy object.



### scheduler.greedy_batching {#scheduler.greedy-batching}

```python
scheduler.greedy_batching(max_concurrent_invocations = None, max_batch_size = None)
```



Shortcut for `scheduler.policy(scheduler.GREEDY_BATCHING_KIND, ...).`

See [scheduler.policy(...)](#scheduler.policy) for all details.

#### Arguments {#scheduler.greedy-batching-args}

* **max_concurrent_invocations**: see [scheduler.policy(...)](#scheduler.policy).
* **max_batch_size**: see [scheduler.policy(...)](#scheduler.policy).




### scheduler.logarithmic_batching {#scheduler.logarithmic-batching}

```python
scheduler.logarithmic_batching(log_base, max_concurrent_invocations = None, max_batch_size = None)
```



Shortcut for `scheduler.policy(scheduler.LOGARITHMIC_BATCHING_KIND, ...)`.

See [scheduler.policy(...)](#scheduler.policy) for all details.

#### Arguments {#scheduler.logarithmic-batching-args}

* **log_base**: see [scheduler.policy(...)](#scheduler.policy). Required.
* **max_concurrent_invocations**: see [scheduler.policy(...)](#scheduler.policy).
* **max_batch_size**: see [scheduler.policy(...)](#scheduler.policy).




### scheduler.newest_first {#scheduler.newest-first}

```python
scheduler.newest_first(max_concurrent_invocations = None, pending_timeout = None)
```



Shortcut for `scheduler.policy(scheduler.NEWEST_FIRST_KIND, ...)`.

See [scheduler.policy(...)](#scheduler.policy) for all details.

#### Arguments {#scheduler.newest-first-args}

* **max_concurrent_invocations**: see [scheduler.policy(...)](#scheduler.policy).
* **pending_timeout**: see [scheduler.policy(...)](#scheduler.policy).






## CQ  {#cq-doc}

CQ module exposes structs and enums useful when defining [luci.cq_group(...)](#luci.cq-group)
entities.

`cq.ACTION_*` constants define possible values for
`allow_owner_if_submittable` field of [luci.cq_group(...)](#luci.cq-group):

  * **cq.ACTION_NONE**: don't grant additional rights to CL owners beyond
    permissions granted based on owner's roles `CQ_COMMITTER` or
    `CQ_DRY_RUNNER` (if any).
  * **cq.ACTION_DRY_RUN** grants the CL owner dry run permission, even if they
    don't have `CQ_DRY_RUNNER` role.
  * **cq.ACTION_COMMIT** grants the CL owner commit and dry run permissions,
    even if they don't have `CQ_COMMITTER` role.

`cq.RETRY_*` constants define some commonly used values for `retry_config`
field of [luci.cq_group(...)](#luci.cq-group):

  * **cq.RETRY_NONE**: never retry any failures.
  * **cq.RETRY_TRANSIENT_FAILURES**: retry only transient (aka "infra")
    failures. Do at most 2 retries across all builders. Each individual
    builder is retried at most once. This is the default.
  * **cq.RETRY_ALL_FAILURES**: retry all failures: transient (aka "infra")
    failures, real test breakages, and timeouts due to lack of available bots.
    For non-timeout failures, do at most 2 retries across all builders. Each
    individual builder is retried at most once. Timeout failures are
    considered "twice as heavy" as non-timeout failures (e.g. one retried
    timeout failure immediately exhausts all retry quota for the CQ attempt).
    This is to avoid adding more requests to an already overloaded system.

`cq.COMMENT_LEVEL_*` constants define possible values for `result_visibility`
field of [luci.cq_tryjob_verifier(...)](#luci.cq-tryjob-verifier):
  * **cq.COMMENT_LEVEL_UNSET**: Equivalent to cq.COMMENT_LEVEL_FULL for now.
  * **cq.COMMENT_LEVEL_FULL**: The CQ reports the summary markdown and a link
    to the buildbucket build id in Milo with the builder name in the URL in a
    Gerrit comment.
  * **cq.COMMENT_LEVEL_RESTRICTED**: The CQ reports a generic "Build failed:
    https://ci.chromium.org/b/1234" with no summary markdown.

`cq.MODE_*` constants define common values for cq run modes.
  * **cq.MODE_DRY_RUN**: Run all tests but do not submit.
  * **cq.MODE_FULL_RUN**: Run all tests and potentially submit.
  * **cq.MODE_NEW_PATCHSET_RUN**: Run tryjobs on patchset upload.

`cq.STATUS_*` constants define possible values for cq run statuses.

`cq.post_action._*` functions construct a post action that performs an action
on a Run completion. They are passed to cq_group() via param `post_actions`.
For exmaple, the following param constructs a post action that votes labels
when a dry-run completes successfully in the cq group.

```python
luci.cq_group(
    name = "main",
    post_actions = [
        cq.post_action_gerrit_label_votes(
            name = "dry-run-verification",
            labels = {"dry-run-succeeded": 1},
            conditions = [cq.post_action_triggering_condition(
               mode = cq.MODE_DRY_RUN,
               statuses = [cq.STATUS_SUCCEEDED],
            )],
        ),
    ],
    ...
)
```


### cq.refset {#cq.refset}

```python
cq.refset(repo, refs = None, refs_exclude = None)
```



Defines a repository and a subset of its refs.

Used in `watch` field of [luci.cq_group(...)](#luci.cq-group) to specify what refs the CQ
should be monitoring.

*** note
**Note:** Gerrit ACLs must be configured such that the CQ has read access to
these refs, otherwise users will be waiting for the CQ to act on their CLs
forever.
***

#### Arguments {#cq.refset-args}

* **repo**: URL of a git repository to watch, starting with `https://`. Only repositories hosted on `*.googlesource.com` are supported currently. Required.
* **refs**: a list of regular expressions that define the set of refs to watch for CLs, e.g. `refs/heads/.+`. If not set, defaults to `refs/heads/main`.
* **refs_exclude**: a list of regular expressions that define the set of refs to exclude from watching. Empty by default.


#### Returns  {#cq.refset-returns}

An opaque struct to be passed to `watch` field of [luci.cq_group(...)](#luci.cq-group).



### cq.retry_config {#cq.retry-config}

```python
cq.retry_config(
    # Optional arguments.
    single_quota = None,
    global_quota = None,
    failure_weight = None,
    transient_failure_weight = None,
    timeout_weight = None,
)
```



Collection of parameters for deciding whether to retry a single build.

All parameters are integers, with default value of 0. The returned struct
can be passed as `retry_config` field to [luci.cq_group(...)](#luci.cq-group).

Some commonly used presents are available as `cq.RETRY_*` constants. See
[CQ](#cq-doc) for more info.

#### Arguments {#cq.retry-config-args}

* **single_quota**: retry quota for a single tryjob.
* **global_quota**: retry quota for all tryjobs in a CL.
* **failure_weight**: the weight assigned to each tryjob failure.
* **transient_failure_weight**: the weight assigned to each transient (aka "infra") failure.
* **timeout_weight**: weight assigned to tryjob timeouts.


#### Returns  {#cq.retry-config-returns}

cq.retry_config struct.



### cq.run_mode {#cq.run-mode}

```python
cq.run_mode(
    # Required arguments.
    name,
    cq_label_value,
    triggering_label,
    triggering_value,
)
```



Defines a CQ Run mode and how it can be triggered.

#### Arguments {#cq.run-mode-args}

* **name**: name of this mode. Must match regex "^[a-zA-Z][a-zA-Z0-9_-]{0,39}$". Required.
* **cq_label_value**: the value of Commit-Queue label that MUST be set to when triggering a CQ Run in this mode. Required.
* **triggering_label**: the Gerrit label that MUST also be set in order to trigger a CQ Run in this mode. Required.
* **triggering_value**: the value of the `triggering_label` that MUST be set to when triggering a CQ Run in this mode. Required.


#### Returns  {#cq.run-mode-returns}

cq.run_mode struct.



### cq.location_filter {#cq.location-filter}

```python
cq.location_filter(
    # Optional arguments.
    gerrit_host_regexp = None,
    gerrit_project_regexp = None,
    gerrit_ref_regexp = None,
    path_regexp = None,
    exclude = None,
)
```



Defines a location filter for the builder location_filters field.

All regexp fields can be empty, which is treated the same as ".*", i.e. a
wildcard which should match anything. Patterns are implicitly wrapped with
"^...$". They are allowed to contain these anchors, but it's redundant.

#### Arguments {#cq.location-filter-args}

* **gerrit_host_regexp**: Gerrit host regex. Must be a valid regex.
* **gerrit_project_regexp**: Gerrit project pattern. Must be a valid regex.
* **gerrit_ref_regexp**: Gerrit ref pattern. Must be a valid regex.
* **path_regexp**: File path pattern. Must be a valid regex.
* **exclude**: Whether this is an "exclude" pattern.


#### Returns  {#cq.location-filter-returns}

cq.location_filter struct.



### cq.post_action_triggering_condition {#cq.post-action-triggering-condition}

```python
cq.post_action_triggering_condition(mode, statuses = None)
```



Constructs [cq.post_action_triggering_condition(...)](#cq.post-action-triggering-condition).

The condition is met if a Run in the mode terminates with one of
  the statuses.

#### Arguments {#cq.post-action-triggering-condition-args}

* **mode**: a Run mode. Could be one of the cq.MODE_* or additional mode name. Required.
* **statuses**: a list of cq.STATUS_*. Required




### cq.post_action_gerrit_label_votes {#cq.post-action-gerrit-label-votes}

```python
cq.post_action_gerrit_label_votes(name, labels, conditions)
```



Constructs a post action that votes Gerrit labels.

#### Arguments {#cq.post-action-gerrit-label-votes-args}

* **name**: the name of the post action. Must be unqiue in scope where is is given. e.g., cg_group. Must match regex '^[0-9A-Za-z][0-9A-Za-z\.\-@_+]{0,511}$'. Required.
* **labels**: a dict of labels to vote. key is the label name in string. value is an int value to vote the label with. Required.
* **conditions**: a list of [cq.post_action_triggering_condition(...)](#cq.post-action-triggering-condition), of which at least one condition has to be met for the action to be executed. Required.




### cq.tryjob_experiment {#cq.tryjob-experiment}

```python
cq.tryjob_experiment(name = None, owner_group_allowlist = None)
```



Constructs an experiment to enable on the Tryjobs.

The experiment will only be enabled if the owner of the CL is a member of
any groups specified in `owner_group_allowlist`.

#### Arguments {#cq.tryjob-experiment-args}

* **name**: name of the experiment. Currently supporting Buildbucket Experiment See `experiments` field in [Builder Config](https://pkg.go.dev/go.chromium.org/luci/buildbucket/proto#BuilderConfig)
* **owner_group_allowlist**: a list of CrIA groups that the owner of the CL must be a member of any group in the list in order to enable the experiment. If None is provided, it means the experiment will always be enabled.




### cq.user_limit {#cq.user-limit}

```python
cq.user_limit(
    # Required arguments.
    name,

    # Optional arguments.
    users = None,
    groups = None,
    run = None,
)
```



Construct a user_limit for run and tryjob limits.

At the time of Run creation, CV looks up a user_limit applicable for
the Run, and blocks processing the Run or the tryjobs, if the number of
ongoing runs and tryjobs reached the limits.

This constructs and return a user_limit, which specifies run and tryjob
limits for given users and members of given groups. Find cq_group(...) to
find how user_limit(s) are used in cq_group(...).

#### Arguments {#cq.user-limit-args}

* **name**: the name of the limit to configure. This usually indicates the intended users and groups. e.g., "limits_for_committers", "limits_for_external_contributors" Must be unique in the ConfigGroup. Must match regex '^[0-9A-Za-z][0-9A-Za-z\.\-@_+]{0,511}$'. Required.
* **users**: a list of user identities to apply the limits to. User identities are the email addresses in most cases.
* **groups**: a list of chrome infra auth groups to apply the limits to the members of.
* **run**: [cq.run_limits(...)](#cq.run-limits). If omitted, runs are unlimited for the users.




### cq.run_limits {#cq.run-limits}

```python
cq.run_limits(max_active = None, reach_limit_msg = None)
```



Constructs Run limits.

All limit values must be > 0, or None if no limit.

#### Arguments {#cq.run-limits-args}

* **max_active**: Max number of ongoing Runs that there can be at any moment.
* **reach_limit_msg**: If set, the value is appended to the message posted to Gerrit when a user hits their run limit.






## Buildbucket




### buildbucket.custom_metric {#buildbucket.custom-metric}

```python
buildbucket.custom_metric(name, predicates, extra_fields = None)
```



Defines a custom metric for builds or builders.

**NOTE**: Before adding a custom metric to your project, you **must** first register it
in [buildbucket's service config](https://chrome-internal.googlesource.com/infradata/config/+/refs/heads/main/configs/cr-buildbucket/settings.cfg)
specifying the metric's name, the standard build metric it bases on and
metric extra_fields, if you plan to add any.

Then when adding the metric to your project, you **must** specify predicates.

* If the metric is to report events of all builds under the builder, use the
  standard build metrics instead.

* If the metric is to report events of all builds under the builder, but with
  extra metric fields, copied from a build metadata, such as tags,
  then add a predicate for the metadata. e.g., `'build.tags.exists(t, t.key=="os")'`.

Elements in the predicates are concatenated with AND during evaluation, meaning
the builds must satisfy all predicates to reported to this custom metrics.
But you could use "||" inside an element to accept a build that satisfys a
portion of that predicate.

Each element must be a boolean expression formatted in
https://github.com/google/cel-spec. Current supported use cases are:
* a build field have non-empty value, e.g.
  * `'has(build.tags)'`
  * `'has(build.input.properties.out_key)'`
* a build string field matchs a value, e.g.
  * `'string(build.output.properties.out_key) == "out_val"'`
* build ends with a specific status: `'build.status.to_string()=="INFRA_FAILURE"'`
* a specific element exists in a repeated field, e.g.
  * `'build.input.experiments.exists(e, e=="luci.buildbucket.exp")'`
  * `'build.steps.exists(s, s.name=="compile")'`
  * `'build.tags.exists(t, t.key=="os")'`
* a build tag has a specific value: `'build.tags.get_value("os")=="Linux"'`

If you specified extra_fields in Buildbucket's service config for the metric,
you **must** also specify them here.
For extra_fields:
* Each key is a metric field.
* Each value must be a string expression on how to generate that field,
  formatted in https://github.com/google/cel-spec. currently supports:
  * status string value: `'build.status.to_string()'`
  * build experiments concatenated with "|": `'build.experiments.to_string()'`
  * value of a build string field, e.g. `'string(build.input.properties.out_key)'`
  * a tag value: `'build.tags.get_value("branch")'`
  * random string literal: `'"m121"'`

Additional metric extra_fields that are not listed in the registration in
buildbucket's service config is allowed, but they will be ignored in
metric reporting until being added to buildbucket's service config.

**NOTE**: if you have a predicate or an extra_fields that cannot be supported by
the CEL expressions we provided, please [file a bug](go/buildbucket-bugs).

**NOTE**: You could test the predicates and extra_fields using the
[CustomMetricPreview](https://cr-buildbucket.appspot.com/rpcexplorer/services/buildbucket.v2.Builds/CustomMetricPreview)
RPC with one example build.

#### Arguments {#buildbucket.custom-metric-args}

* **name**: custom metric name. It must be a pre-registered custom metric in buildbucket's service config. Required.
* **predicates**: a list of strings with CEL predicate expressions. Required.
* **extra_fields**: a string dict with CEL expression for generating metric field values.


#### Returns  {#buildbucket.custom-metric-returns}

buildbucket.custom_metric struct with fields `name`, `predicates` and `extra_fields`.



### buildbucket.validate_custom_metrics {#buildbucket.validate-custom-metrics}

```python
buildbucket.validate_custom_metrics(attr, custom_metrics = None)
```



Validates a list of custom metrics.

Ensure each entry is buildbucket.custom_metric struct.

#### Arguments {#buildbucket.validate-custom-metrics-args}

* **attr**: field name with settings, for error messages. Required.
* **custom_metrics**: A list of buildbucket_pb.CustomMetricDefinition() protos.


#### Returns  {#buildbucket.validate-custom-metrics-returns}

Validates list of custom metrics (may be an empty list, never None).





## Built-in constants and functions

Refer to the list of [built-in constants and functions][starlark-builtins]
exposed in the global namespace by Starlark itself.

[starlark-builtins]: https://github.com/google/starlark-go/blob/master/doc/spec.md#built-in-constants-and-functions

In addition, `lucicfg` exposes the following functions.





### __load {#load}

```python
__load(module, *args, **kwargs)
```



Loads a Starlark module as a library (if it hasn't been loaded before).

Extracts one or more values from it, and binds them to names in the current
module.

A load statement requires at least two "arguments". The first must be a
literal string, it identifies the module to load. The remaining arguments
are a mixture of literal strings, such as `'x'`, or named literal strings,
such as `y='x'`.

The literal string (`'x'`), which must denote a valid identifier not
starting with `_`, specifies the name to extract from the loaded module. In
effect, names starting with `_` are not exported. The name (`y`) specifies
the local name. If no name is given, the local name matches the quoted name.

```
load('//module.star', 'x', 'y', 'z')       # assigns x, y, and z
load('//module.star', 'x', y2='y', 'z')    # assigns x, y2, and z
```

A load statement within a function is a static error.

See also [Modules and packages](#modules-and-packages) for how load(...)
interacts with [exec(...)](#exec).

#### Arguments {#load-args}

* **module**: module to load, i.e. `//path/within/current/package.star` or `@<pkg>//path/within/pkg.star` or `./relative/path.star`. Required.
* **\*args**: what values to import under their original names.
* **\*\*kwargs**: what values to import and bind under new names.




### exec {#exec}

```python
exec(module)
```



Executes another Starlark module for its side effects.

See also [Modules and packages](#modules_and_packages) for how load(...)
interacts with [exec(...)](#exec).

#### Arguments {#exec-args}

* **module**: module to execute, i.e. `//path/within/current/package.star` or `@<pkg>//path/within/pkg.star` or `./relative/path.star`. Required.


#### Returns  {#exec-returns}

A struct with all exported symbols of the executed module.



### fail {#fail}

```python
fail(msg, trace = None)
```



Aborts the execution with an error message.

#### Arguments {#fail-args}

* **msg**: the error message string. Required.
* **trace**: a custom trace, as returned by [stacktrace(...)](#stacktrace) to attach to the error. This may be useful if the root cause of the error is far from where `fail` is called.




### stacktrace {#stacktrace}

```python
stacktrace(skip = None)
```



Captures and returns a stack trace of the caller.

A captured stacktrace is an opaque object that can be stringified to get a
nice looking trace (e.g. for error messages).

#### Arguments {#stacktrace-args}

* **skip**: how many innermost call frames to skip. Default is 0.




### struct {#struct}

```python
struct(**kwargs)
```



Returns an immutable struct object with given fields.

Can be used to define namespaces, for example:

```python
def _func1():
    ...

def _func2():
    ...

exported = struct(
    func1 = _func1,
    func2 = _func2,
)
```

Then `_func1` can be called as `exported.func1()`.

#### Arguments {#struct-args}

* **\*\*kwargs**: fields to put into the returned struct object.




### to_json {#to-json}

```python
to_json(value)
```



Serializes a value to a compact JSON string.

Doesn't support integers that do not fit int64. Fails if the value has
cycles.

*** note
**Deprecated.** Use [json.encode(...)](#json.encode) instead. Note that [json.encode(...)](#json.encode)
will retain the order of dict keys, unlike [to_json(...)](#to-json) that always sorts
them alphabetically.
***

#### Arguments {#to-json-args}

* **value**: a primitive Starlark value: a scalar, or a list/tuple/dict containing only primitive Starlark values. Required.









### json.encode {#json.encode}

```python
json.encode(value)
```



Encodes a value into a JSON string.

Accepts one required positional argument, which it converts to JSON by
cases:

  - None, True, and False are converted to null, true, and false,
    respectively.
  - Starlark int values, no matter how large, are encoded as decimal
    integers. Some decoders may not be able to decode very large integers.
  - Starlark float values are encoded using decimal point notation, even if
    the value is an integer. It is an error to encode a non-finite
    floating-point value.
  - Starlark strings are encoded as JSON strings, using UTF-16 escapes.
  - a Starlark IterableMapping (e.g. dict) is encoded as a JSON object.
    It is an error if any key is not a string. The order of keys is
    retained.
  - any other Starlark Iterable (e.g. list, tuple) is encoded as a JSON
    array.
  - a Starlark HasAttrs (e.g. struct) is encoded as a JSON object.

Encoding any other value yields an error.

#### Arguments {#json.encode-args}

* **value**: a value to encode. Required.




### json.decode {#json.decode}

```python
json.decode(str)
```



Decodes a JSON string.

Accepts one positional parameter, a JSON string. It returns the Starlark
value that the string denotes:

  - Numbers are parsed as int or float, depending on whether they contain
    a decimal point.
  - JSON objects are parsed as new unfrozen Starlark dicts.
  - JSON arrays are parsed as new unfrozen Starlark lists.

Decoding fails if `str` is not a valid JSON string.

#### Arguments {#json.decode-args}

* **str**: a JSON string to decode. Required.




### json.indent {#json.indent}

```python
json.indent(str, prefix = None, indent = None)
```



Pretty-prints a valid JSON encoding.

#### Arguments {#json.indent-args}

* **str**: the JSON string to pretty-print. Required.
* **prefix**: a prefix of each new line.
* **indent**: a unit of indentation.


#### Returns  {#json.indent-returns}

The indented form of `str`.








### proto.to_textpb {#proto.to-textpb}

```python
proto.to_textpb(msg)
```



Serializes a protobuf message to a string using ASCII proto serialization.

#### Arguments {#proto.to-textpb-args}

* **msg**: a proto message to serialize. Required.




### proto.to_jsonpb {#proto.to-jsonpb}

```python
proto.to_jsonpb(msg, use_proto_names = None)
```



Serializes a protobuf message to a string using JSONPB serialization.

#### Arguments {#proto.to-jsonpb-args}

* **msg**: a proto message to serialize. Required.
* **use_proto_names**: boolean, whether to use snake_case in field names instead of camelCase. The default is False.




### proto.to_wirepb {#proto.to-wirepb}

```python
proto.to_wirepb(msg)
```



Serializes a protobuf message to a string using binary wire encoding.

#### Arguments {#proto.to-wirepb-args}

* **msg**: a proto message to serialize. Required.




### proto.from_textpb {#proto.from-textpb}

```python
proto.from_textpb(ctor, text, discard_unknown = None)
```



Deserializes a protobuf message given its ASCII proto serialization.

The returned message is frozen. Use [proto.clone(...)](#proto.clone) to get a mutable
copy if necessary.

#### Arguments {#proto.from-textpb-args}

* **ctor**: a message constructor function, the same one you would normally use to create a new message. Required.
* **text**: a string with the serialized message. Required.
* **discard_unknown**: boolean, whether to discard unrecognized fields. The default is False.


#### Returns  {#proto.from-textpb-returns}

Deserialized frozen message constructed via `ctor`.



### proto.from_jsonpb {#proto.from-jsonpb}

```python
proto.from_jsonpb(ctor, text, discard_unknown = None)
```



Deserializes a protobuf message given its JSONPB serialization.

The returned message is frozen. Use [proto.clone(...)](#proto.clone) to get a mutable
copy if necessary.

#### Arguments {#proto.from-jsonpb-args}

* **ctor**: a message constructor function, the same one you would normally use to create a new message. Required.
* **text**: a string with the serialized message. Required.
* **discard_unknown**: boolean, whether to discard unrecognized fields. The default is True.


#### Returns  {#proto.from-jsonpb-returns}

Deserialized frozen message constructed via `ctor`.



### proto.from_wirepb {#proto.from-wirepb}

```python
proto.from_wirepb(ctor, blob, discard_unknown = None)
```



Deserializes a protobuf message given its wire serialization.

The returned message is frozen. Use [proto.clone(...)](#proto.clone) to get a mutable
copy if necessary.

#### Arguments {#proto.from-wirepb-args}

* **ctor**: a message constructor function, the same one you would normally use to create a new message. Required.
* **blob**: a string with the serialized message. Required.
* **discard_unknown**: boolean, whether to discard unrecognized fields. The default is True.


#### Returns  {#proto.from-wirepb-returns}

Deserialized frozen message constructed via `ctor`.



### proto.struct_to_textpb {#proto.struct-to-textpb}

```python
proto.struct_to_textpb(s = None)
```



Converts a struct to a text proto string.

#### Arguments {#proto.struct-to-textpb-args}

* **s**: a struct object. May not contain dicts.


#### Returns  {#proto.struct-to-textpb-returns}

A str containing a text format protocol buffer message.



### proto.clone {#proto.clone}

```python
proto.clone(msg)
```



Returns a deep copy of a given proto message.

#### Arguments {#proto.clone-args}

* **msg**: a proto message to make a copy of. Required.


#### Returns  {#proto.clone-returns}

A deep copy of the message.



### proto.has {#proto.has}

```python
proto.has(msg, field)
```



Checks if a proto message has the given optional field set.

Following rules apply:

  * Fields that are not defined in the `*.proto` file are always unset.
  * Singular fields of primitive types (e.g. `int64`), repeated and map
    fields (even empty ones) are always set. There's no way to distinguish
    zero values of such fields from unset fields.
  * Singular fields of message types are set only if they were explicitly
    initialized (e.g. by writing to such field or reading a default value
    from it).
  * Alternatives of a `oneof` field (regardless of their type) are
    initialized only when they are explicitly "picked".

#### Arguments {#proto.has-args}

* **msg**: a message to check. Required.
* **field**: a string name of the field to check. Required.


#### Returns  {#proto.has-returns}

True if the message has the field set.








### io.read_file {#io.read-file}

```python
io.read_file(path)
```



Reads a file and returns its contents as a string.

Useful for rules that accept large chunks of free form text. By using
`io.read_file` such text can be kept in a separate file.

#### Arguments {#io.read-file-args}

* **path**: either a path relative to the currently executing Starlark script, or (if starts with `//`) an absolute path within the currently executing package. If it is a relative path, it must point somewhere inside the current package directory. Required.


#### Returns  {#io.read-file-returns}

The contents of the file as a string. Fails if there's no such file, it
can't be read, or it is outside of the current package directory.



### io.read_proto {#io.read-proto}

```python
io.read_proto(ctor, path, encoding = None)
```



Reads a serialized proto message from a file, deserializes and returns it.

#### Arguments {#io.read-proto-args}

* **ctor**: a constructor function that defines the message type. Required.
* **path**: either a path relative to the currently executing Starlark script, or (if starts with `//`) an absolute path within the currently executing package. If it is a relative path, it must point somewhere inside the current package directory. Required.
* **encoding**: either `jsonpb` or `textpb` or `auto` to detect based on the file extension. Default is `auto`.


#### Returns  {#io.read-proto-returns}

Deserialized proto message constructed via `ctor`.






## (Experimental) Functions only available inside PACKAGE.star {#lucicfg-package}






### pkg.declare {#pkg.declare}

```python
pkg.declare(name, lucicfg)
```



Declares a lucicfg package.

It sets the package import name and the required minimum version of lucicfg.

The name must start with `@`: all load statements referring to this package
from other packages will use this name:

```
load("@importname//path/within/the/package.star", ...)
```

See [Modules and packages](#modules-and-packages) for more details.

This name should be reasonably unique: it is impossible to use two
different packages with the same name as dependencies in a single
dependency tree (even if they are indirect dependencies).

For repos containing recipes, this should be the same value as the recipe
repo-name for consistency, though to avoid recipe<->lucicfg entanglement,
this is not a requirement.

This statement is required exactly once and it must be the first statement
in PACKAGE.star file.

The names `@stdlib`, `@__main__` and `@proto` are reserved.

#### Arguments {#pkg.declare-args}

* **name**: the name for this package. Required.
* **lucicfg**: a string `major.minor.revision` with a minimum lucicfg version this package requires. Required.




### pkg.depend {#pkg.depend}

```python
pkg.depend(name, source)
```



Declares a dependency on another lucicfg package.

This essentially declares that `load("@<name>//<path>", ...)` statements
should be resolved to files from the given source at a revision no older
than the one specified in the [pkg.source.googlesource(...)](#pkg.source.googlesource) declaration.

The name must match the name declared by the dependent package itself in
its [pkg.declare(...)](#pkg.declare) statement.

Packages form a dependency DAG. All dependencies with a given name should
all be fetched from the same source (the same repo and the same ref), but
perhaps have different minimal version constraints. The final single version
of the dependency that will be used by all packages in the DAG is picked as
a newest among all collected minimal version constraints (using the git
ref's history for finding which version is newer).

The resolved versions are written into a lock file of the current package
(`PACKAGE.lock`, which is a JSON file). This lock file is used exclusively
when this package is the entry point package for some lucicfg execution. In
other words, lock files of imported dependencies have no effect on the
dependency resolution process at all.

The lock file is required to run `lucicfg generate`. It can be generated by
`lucicfg lock` subcommand and checked in into the repository with the rest
of the package. It is validated as part of `lucicfg validate` call.

#### Arguments {#pkg.depend-args}

* **name**: the name of the depended package. Required.
* **source**: a pkg.source.ref struct as produced by [pkg.source.googlesource(...)](#pkg.source.googlesource), [pkg.source.submodule(...)](#pkg.source.submodule) or [pkg.source.local(...)](#pkg.source.local). Required.




### pkg.resources {#pkg.resources}

```python
pkg.resources(patterns = None)
```



Declares non-Starlark files to includes into the package.

Only these files can be read at runtime via [io.read_file(...)](#io.read-file) or
[io.read_proto(...)](#io.read-proto).

Declaring them upfront is useful when the package is used as a dependency.
Resource files are prefetched from the package source.

Can be called multiple times. Works additively.

#### Arguments {#pkg.resources-args}

* **patterns**: a list of glob patterns that define a subset of non-Starlark files under the package directory. Each entry is either `<glob pattern>` (a "positive" glob) or `!<glob pattern>` (a "negative" glob). A file is considered to be a resource file if its slash-separated path matches any of the positive globs and none of the negative globs. If a pattern starts with `**/`, the rest of it is applied to the base name of the file (not the whole path). If only negative globs are given, a single positive `**/*` glob is implied as well.




### pkg.entrypoint {#pkg.entrypoint}

```python
pkg.entrypoint(path = None)
```



Declares that the given Starlark file is one of the entry point scripts.

Entry point scripts are scripts that can be executed (via
`lucicfg gen <path>`) to generate some configuration file. Only entry point
scripts can be executed.

#### Arguments {#pkg.entrypoint-args}

* **path**: a path to a Starlark file relative to the package root.









### pkg.source.googlesource {#pkg.source.googlesource}

```python
pkg.source.googlesource(
    # Required arguments.
    host,
    repo,
    ref,
    path,
    minimum_version,
)
```



Defines a reference to package source stored in a googlesource.com repo.

#### Arguments {#pkg.source.googlesource-args}

* **host**: a googlesource.com source host name (e.g. `chromium`). Required.
* **repo**: a name of the repository on the host (e.g. `chromium/src`). Required.
* **ref**: a full git reference (e.g. `refs/heads/main`) to fetch. The history of this reference is used to determine the ordering of commits when resolving versions of dependencies. Required.
* **path**: a directory path to the lucicfg package root (a directory with PACKAGE.star file) within the source repo. Required.
* **minimum_version**: a full git commit hash with a minimum compatible version of this dependency. In the final resolved dependency set, the dependency will be at this revision or newer (in case some other package depends on a newer version). Must be reachable from the given git ref. Required.


#### Returns  {#pkg.source.googlesource-returns}

A pkg.source.ref struct that can be passed to [pkg.depend(...)](#pkg.depend).



### pkg.source.submodule {#pkg.source.submodule}

```python
pkg.source.submodule(path)
```



Builds a reference to package source by reading a git submodule.

Works relative to the repository of the package that declared the
dependency (aka "the current package repository").

Constructs [pkg.source.googlesource(...)](#pkg.source.googlesource) by reading the checked in
`.gitmodules` file of the current package repository to figure out the host,
the repo, the ref and the minimum version of the dependency identified by
the given path.

Recursive submodules are not supported.

#### Arguments {#pkg.source.submodule-args}

* **path**: a relative path from the current package directory to the directory with the target dependency (i.e. the directory that contains PACKAGE.star file). This directory must be within some git submodule path. Required.


#### Returns  {#pkg.source.submodule-returns}

A pkg.source.ref struct that can be passed to [pkg.depend(...)](#pkg.depend).



### pkg.source.local {#pkg.source.local}

```python
pkg.source.local(path)
```



Builds a reference to package source stored in the current repository.

Works relative to the repository of the package that declared the
dependency (aka "the current package repository").

Constructs [pkg.source.googlesource(...)](#pkg.source.googlesource) by taking the source reference of
the current package and replacing the path there to point to another
package.

#### Arguments {#pkg.source.local-args}

* **path**: a relative path from the current package directory to the directory (within the same repository) with the target dependency. Required.


#### Returns  {#pkg.source.local-returns}

A pkg.source.ref struct that can be passed to [pkg.depend(...)](#pkg.depend).








### pkg.options.lint_checks {#pkg.options.lint-checks}

```python
pkg.options.lint_checks(checks = None)
```



Configures linting rules that apply to files in this package.

Can be called at most once.

#### Arguments {#pkg.options.lint-checks-args}

* **checks**: a list of linter checks to apply in `lucicfg validate` and `lucicfg lint`. The first entry defines what group of checks to use as a base and it can be one of `none`, `default` or `all`. The following entries either add checks to the set (`+<name>`) or remove them (`-<name>`). See [Formatting and linting Starlark code](#formatting-linting) for more info. Default is `['none']` for now.




### pkg.options.fmt_rules {#pkg.options.fmt-rules}

```python
pkg.options.fmt_rules(paths, function_args_sort = None)
```



Adds a formatting rule set applying to some paths in the package.

When processing files, lucicfg will select a single rule set based on the
longest matching rule's path prefix. For example, if there are two rule
sets, one formatting "a" and another formatting "a/folder", then for the
file "a/folder/file.star", only the second rules set would apply. If NO
rules set matches the file path, then only default formatting will occur.

#### Arguments {#pkg.options.fmt-rules-args}

* **paths**: forward-slash delimited path prefixes for which this rule set applies. Rules with duplicate path values are not permitted (i.e. you cannot have two rules with a path of "something", nor can you have the path "something" duplicated within a single rule). Required.
* **function_args_sort**: if set, specifies how to sort keyword argument in function calls. Should be a list of strings (perhaps empty). Keyword arguments in all function calls will be ordered based on the order in this list. Arguments that do not appear in the list, will be sorted alphanumerically and put after all arguments in the list. This implies that passing an empty list will result in sorting all keyword arguments in all function calls alphanumerically. Optional.





