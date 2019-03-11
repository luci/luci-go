# LUCI configuration definition language






















[TOC]

## Overview

### Working with lucicfg

*** note
TODO: To be written.
***


### Concepts

*** note
TODO: To be written.
***


### Execution model {#execution_doc}

*** note
TODO: To be written.
***


### Resolving naming ambiguities

*** note
TODO: To be written.
***


### Defining cron schedules {#schedules_doc}

*** note
TODO: To be written.
***


## Interfacing with lucicfg internals




### lucicfg.version {#lucicfg.version}

```python
lucicfg.version()
```



Returns a triple with lucicfg version: `(major, minor, revision)`.





### lucicfg.config {#lucicfg.config}

```python
lucicfg.config(
    # Optional arguments.
    config_service_host = None,
    config_set = None,
    config_dir = None,
    tracked_files = None,
    fail_on_warnings = None,
)
```



Sets one or more parameters for the `lucicfg` itself.

These parameters do not affect semantic meaning of generated configs, but
influence how they are generated and validated.

Each parameter has a corresponding command line flag. If the flag is present,
it overrides the value set via `lucicfg.config` (if any). For example, the
flag `-config-service-host <value>` overrides whatever was set via
`lucicfg.config(config_service_host=...)`.

`lucicfg.config` is allowed to be called multiple times. The most recently set
value is used in the end, so think of `lucicfg.config(var=...)` just as
assigning to a variable.

#### Arguments {#lucicfg.config-args}

* **config_service_host**: a hostname of a LUCI Config Service to send validation requests to. Default is whatever is hardcoded in `lucicfg` binary, usually `luci-config.appspot.com`.
* **config_set**: name of the config set in LUCI Config Service to use for validation. Default is `projects/<name>` where `<name>` is taken from [luci.project(...)](#luci.project) rule. If there's no such rule, the default is "", meaning the generated config will not be validated via LUCI Config Service.
* **config_dir**: a directory to place generated configs into, relative to the directory that contains the entry point \*.star file. `..` is allowed. If set via `-config-dir` command line flag, it is relative to the current working directory. Will be created if absent. If `-`, the configs are just printed to stdout in a format useful for debugging. Default is "generated".
* **tracked_files**: a list of glob patterns that define a subset of files under `config_dir` that are considered generated. Each entry is either `<glob pattern>` (a "positive" glob) or `!<glob pattern>` (a "negative" glob). If only negative globs are given, single positive `*` glob is implied as well. A file under `config_dir` (or any of its subdirectories) is considered tracked if its base name (not path!) matches any of the positive globs and none of the negative globs. `tracked_files` can be used to limit what files are actually written to disk: if this set is not empty, only files that are in this set will be actually written to disk (and all other files are discarded). This is beneficial when `lucicfg` is used to generate only a subset of config files, e.g. during the  migration from handcrafted to generated configs. Knowing the tracked files set is also important when some generated file disappears from `lucicfg` output: it must be deleted from the disk as well. To do this, `lucicfg` needs to know what files are safe to delete. If `tracked_files` is empty (default), `lucicfg` will save all generated files and will never delete any file (in this case it is responsibility of the caller to make sure no stale output remains).
* **fail_on_warnings**: if set to True treat validation warnings as errors. Default is False (i.e. warnings do to cause the validation to fail). If set to True via `lucicfg.config` and you want to override it to False via command line flags use `-fail-on-warnings=false`.




### lucicfg.generator {#lucicfg.generator}

```python
lucicfg.generator(impl = None)
```


*** note
**Advanced function.** It is not used for common use cases.
***


Registers a callback that is called at the end of the config generation
stage to modify/append/delete generated configs in an arbitrary way.

The callback accepts single argument `ctx` which is a struct with the
following fields:

  * **config_set**: a dict `{config file name -> (str | proto)}`.

The callback is free to modify `ctx.config_set` in whatever way it wants, e.g.
by adding new values there or mutating/deleting existing ones.

#### Arguments {#lucicfg.generator-args}

* **impl**: a callback `func(ctx) -> None`.




### lucicfg.var {#lucicfg.var}

```python
lucicfg.var(default = None, validator = None)
```


*** note
**Advanced function.** It is not used for common use cases.
***


Declares a variable.

A variable is a slot that can hold some frozen value. Initially this slot is
empty. [lucicfg.var(...)](#lucicfg.var) returns a struct with methods to manipulate this slot:

  * `set(value)`: sets the variable's value if it's unset, fails otherwise.
  * `get()`: returns the current value, auto-setting it to `default` if it was
    unset.

Note the auto-setting the value in `get()` means once `get()` is called on an
unset variable, this variable can't be changed anymore, since it becomes
initialized and initialized variables are immutable. In effect, all callers of
`get()` within a scope always observe the exact same value (either an
explicitly set one, or a default one).

Any module (loaded or exec'ed) can declare variables via [lucicfg.var(...)](#lucicfg.var). But
only modules running through [exec(...)](#exec) can read and write them. Modules being
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

Generator callbacks registered via [lucicfg.generator(...)](#lucicfg.generator) are forbidden to
read or write variables, since they execute outside of context of any
[exec(...)](#exec). Generators must operate exclusively over state stored in the node
graph. Note that variables still can be used by functions that _build_ the
graph, they can transfer information from variables into the graph, if
necessary.

The most common application for [lucicfg.var(...)](#lucicfg.var) is to "configure" library
modules with default values pertaining to some concrete executing script:

  * A library declares variables while it loads and exposes them in its public
    API either directly or via wrapping setter functions.
  * An executing script uses library's public API to set variables' values to
    values relating to what this script does.
  * All calls made to the library from the executing script (or any scripts it
    includes with [exec(...)](#exec)) can access variables' values now.

This is more magical but less wordy alternative to either passing specific
default values in every call to library functions, or wrapping all library
functions with wrappers that supply such defaults. These more explicit
approaches can become pretty convoluted when there are multiple scripts and
libraries involved.

#### Arguments {#lucicfg.var-args}

* **default**: a value to auto-set to the variable in `get()` if it was unset.
* **validator**: a callback called as `validator(value)` from `set(value)`, must return the value to be assigned to the variable (usually just `value` itself).


#### Returns  {#lucicfg.var-returns}

A struct with two methods: `set(value)` and `get(): value`.



### lucicfg.rule {#lucicfg.rule}

```python
lucicfg.rule(impl)
```


*** note
**Advanced function.** It is not used for common use cases.
***


Declares a new rule.

A rule is a callable that adds nodes and edges to an entity graph.

#### Arguments {#lucicfg.rule-args}

* **impl**: a callback that actually implements the rule. Required.


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



### time.days_of_week {#time.days_of_week}

```python
time.days_of_week(spec)
```



Parses e.g. `Tue,Fri-Sun` into a list of day indexes, e.g. `[2, 5, 6, 7]`.

Monday is 1, Sunday is 7. The returned list is sorted and has no duplicates.
An empty string results in the empty list.

#### Arguments {#time.days_of_week-args}

* **spec**: a case-insensitive string with 3-char abbreviated days of the week. Multiple terms are separated by a comma and optional spaces. Each term is either a day (e.g. `Tue`), or a range (e.g. `Wed-Sun`). Required.


#### Returns  {#time.days_of_week-returns}

A list of 1-based day indexes. Monday is 1.





## Core LUCI rules




### luci.project {#luci.project}

```python
luci.project(
    # Required arguments.
    name,

    # Optional arguments.
    buildbucket = None,
    logdog = None,
    milo = None,
    scheduler = None,
    swarming = None,
    acls = None,
)
```



Defines a LUCI project.

There should be exactly one such definition in the top-level config file.

#### Arguments {#luci.project-args}

* **name**: full name of the project. Required.
* **buildbucket**: appspot hostname of a Buildbucket service to use (if any).
* **logdog**: appspot hostname of a LogDog service to use (if any).
* **milo**: appspot hostname of a Milo service to use (if any).
* **scheduler**: appspot hostname of a LUCI Scheduler service to use (if any).
* **swarming**: appspot hostname of a Swarming service to use (if any).
* **acls**: list of [acl.entry(...)](#acl.entry) objects, will be inherited by all buckets.




### luci.logdog {#luci.logdog}

```python
luci.logdog(gs_bucket = None)
```



Defines configuration of the LogDog service for this project.

Usually required for any non-trivial project.

#### Arguments {#luci.logdog-args}

* **gs_bucket**: base Google Storage archival path, archive logs will be written to this bucket/path.




### luci.bucket {#luci.bucket}

```python
luci.bucket(name, acls = None)
```



Defines a bucket: a container for LUCI resources that share the same ACL.

#### Arguments {#luci.bucket-args}

* **name**: name of the bucket, e.g. `ci` or `try`. Required.
* **acls**: list of [acl.entry(...)](#acl.entry) objects.




### luci.recipe {#luci.recipe}

```python
luci.recipe(
    # Required arguments.
    name,
    cipd_package,

    # Optional arguments.
    cipd_version = None,
    recipe = None,
)
```



Defines where to locate a particular recipe.

Builders refer to recipes in their `recipe` field, see [luci.builder(...)](#luci.builder).
Multiple builders can execute the same recipe (perhaps passing different
properties to it).

Recipes are located inside cipd packages called "recipe bundles". Typically
the cipd package name with the recipe bundle will look like:

    infra/recipe_bundles/chromium.googlesource.com/chromium/tools/build

Recipes bundled from internal repositories are typically under

    infra_internal/recipe_bundles/...

But if you're building your own recipe bundles, they could be located
elsewhere.

The cipd version to fetch is usually a lower-cased git ref (like
`refs/heads/master`), or it can be a cipd tag (like `git_revision:abc...`).

#### Arguments {#luci.recipe-args}

* **name**: name of this recipe entity, to refer to it from builders. If `recipe` is None, also specifies the recipe name within the bundle. Required.
* **cipd_package**: a cipd package name with the recipe bundle. Required.
* **cipd_version**: a version of the recipe bundle package to fetch, default is `refs/heads/master`.
* **recipe**: name of a recipe inside the recipe bundle if it differs from `name`. Useful if recipe names clash between different recipe bundles. When this happens, `name` can be used as a non-ambiguous alias, and `recipe` can provide the actual recipe name. Defaults to `name`.




### luci.builder {#luci.builder}

```python
luci.builder(
    # Required arguments.
    name,
    bucket,
    recipe,

    # Optional arguments.
    properties = None,
    service_account = None,
    caches = None,
    execution_timeout = None,
    dimensions = None,
    priority = None,
    swarming_tags = None,
    expiration_timeout = None,
    schedule = None,
    triggering_policy = None,
    build_numbers = None,
    experimental = None,
    task_template_canary_percentage = None,
    luci_migration_host = None,
    triggers = None,
    triggered_by = None,
)
```



Defines a generic builder.

It runs some recipe in some requested environment, passing it a struct with
given properties. It is launched whenever something triggers it (a poller or
some other builder, or maybe some external actor via Buildbucket or LUCI
Scheduler APIs).

The full unique builder name (as expected by Buildbucket RPC interface) is
a pair `(<project>, <bucket>/<name>)`, but within a single project config
this builder can be referred to either via its bucket-scoped name (i.e.
`<bucket>/<name>`) or just via it's name alone (i.e. `<name>`), if this
doesn't introduce ambiguities.

The definition of what can *potentially* trigger what is defined through
`triggers` and `triggered_by` fields. They specify how to prepare ACLs and
other configuration of services that execute builds. If builder **A** is
defined as "triggers builder **B**", it means all services should expect **A**
builds to trigger **B** builds via LUCI Scheduler's EmitTriggers RPC or via
Buildbucket's ScheduleBuild RPC, but the actual triggering is still the
responsibility of **A**'s recipe.

There's a caveat though: only Scheduler ACLs are auto-generated by the config
generator when one builder triggers another, because each Scheduler job has
its own ACL and we can precisely configure who's allowed to trigger this job.
Buildbucket ACLs are left unchanged, since they apply to an entire bucket, and
making a large scale change like that (without really knowing whether
Buildbucket API will be used) is dangerous. If the recipe triggers other
builds directly through Buildbucket, it is the responsibility of the config
author (you) to correctly specify Buildbucket ACLs, for example by adding the
corresponding service account to the bucket ACLs:

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

This is not necessary if the recipe uses Scheduler API instead of Buildbucket.

#### Arguments {#luci.builder-args}

* **name**: name of the builder, will show up in UIs and logs. Required.
* **bucket**: a bucket the builder is in, see [luci.bucket(...)](#luci.bucket) rule. Required.
* **recipe**: a recipe to run, see [luci.recipe(...)](#luci.recipe) rule. Required.
* **properties**: a dict with string keys and JSON-serializable values, defining properties to pass to the recipe.
* **service_account**: an email of a service account to run the recipe under: the recipe (and various tools it calls, e.g. gsutil) will be able to make outbound HTTP calls that have an OAuth access token belonging to this service account (provided it is registered with LUCI).
* **caches**: a list of [swarming.cache(...)](#swarming.cache) objects describing Swarming named caches that should be present on the bot. See [swarming.cache(...)](#swarming.cache) doc for more details.
* **execution_timeout**: how long to wait for a running build to finish before forcefully aborting it and marking the build as timed out. If None, defer the decision to Buildbucket service.
* **dimensions**: a dict with swarming dimensions, indicating requirements for a bot to execute the build. Keys are strings (e.g. `os`), and values are either strings (e.g. `Linux`), [swarming.dimension(...)](#swarming.dimension) objects (for defining expiring dimensions) or lists of thereof.
* **priority**: int [1-255] or None, indicating swarming task priority, lower is more important. If None, defer the decision to Buildbucket service.
* **swarming_tags**: a list of tags (`k:v` strings) to assign to the Swarming task that runs the builder. Each tag will also end up in `swarming_tag` Buildbucket tag, for example `swarming_tag:builder:release`.
* **expiration_timeout**: how long to wait for a build to be picked up by a matching bot (based on `dimensions`) before canceling the build and marking it as expired. If None, defer the decision to Buildbucket service.
* **schedule**: string with a cron schedule that describes when to run this builder. See [Defining cron schedules](#schedules_doc) for the expected format of this field. If None, the builder will not be running periodically.
* **triggering_policy**: [scheduler.policy(...)](#scheduler.policy) struct with a configuration that defines when and how LUCI Scheduler should launch new builds in response to triggering requests from [luci.gitiles_poller(...)](#luci.gitiles_poller) or from EmitTriggers API. Does not apply to builds started directly through Buildbucket. By default, only one concurrent build is allowed and while it runs, triggering requests accumulate in a queue. Once the build finishes, if the queue is not empty, a new build starts right away, "consuming" all pending requests. See [scheduler.policy(...)](#scheduler.policy) doc for more details.
* **build_numbers**: if True, generate monotonically increasing contiguous numbers for each build, unique within the builder. If None, defer the decision to Buildbucket service.
* **experimental**: if True, by default a new build in this builder will be marked as experimental. This is seen from recipes and they may behave differently (e.g. avoiding any side-effects). If None, defer the decision to Buildbucket service.
* **task_template_canary_percentage**: int [0-100] or None, indicating percentage of builds that should use a canary swarming task template. If None, defer the decision to Buildbucket service.
* **luci_migration_host**: deprecated setting that was important during the migration from Buildbot to LUCI. Refer to Buildbucket docs for the meaning.
* **triggers**: builders this builder triggers.
* **triggered_by**: builders or pollers this builder is triggered by.




### luci.gitiles_poller {#luci.gitiles_poller}

```python
luci.gitiles_poller(
    # Required arguments.
    name,
    bucket,
    repo,

    # Optional arguments.
    refs = None,
    schedule = None,
    triggers = None,
)
```



Defines a gitiles poller which can trigger builders on git commits.

It periodically examines the state of watched refs in the git repository. On
each iteration it triggers builders if either:

  * A watched ref's tip has changed since the last iteration (e.g. a new
    commit landed on a ref). Each new detected commit results in a separate
    triggering request, so if for example 10 new commits landed on a ref since
    the last poll, 10 new triggering requests will be submitted to the
    builders triggered by this poller. How they are converted to actual builds
    depends on `triggering_policy` of a builder. For example, some builders
    may want to have one build per commit, others don't care and just want to
    test the latest commit. See [luci.builder(...)](#luci.builder) and [scheduler.policy(...)](#scheduler.policy)
    for more details.

    *** note
    **Caveat**: When a large number of commits are pushed on the ref between
    iterations of the poller, only the most recent 50 commits will result in
    triggering requests. Everything older is silently ignored. This is a
    safeguard against mistaken or deliberate but unusual git push actions,
    which typically don't have intent of triggering a build for each such
    commit.
    ***

  * A ref belonging to the watched set has just been created. This produces
    a single triggering request.

#### Arguments {#luci.gitiles_poller-args}

* **name**: name of the poller, to refer to it from other rules. Required.
* **bucket**: a bucket the poller is in, see [luci.bucket(...)](#luci.bucket) rule. Required.
* **repo**: URL of a git repository to poll, starting with `https://`. Required.
* **refs**: a list of regular expressions that define the watched set of refs, e.g. `refs/heads/[^/]+` or `refs/branch-heads/\d+\.\d+`. The regular expression should have a literal prefix with at least two slashes present, e.g. `refs/release-\d+/foobar` is *not allowed*, because the literal prefix `refs/release-` contains only one slash. The regexp should not start with `^` or end with `$` as they will be added automatically. If empty, defaults to `['refs/heads/master']`.
* **schedule**: string with a schedule that describes when to run one iteration of the poller. See [Defining cron schedules](#schedules_doc) for the expected format of this field. Note that it is rare to use custom schedules for pollers. By default, the poller will run each 30 sec.
* **triggers**: builders to trigger whenever the poller detects a new git commit on any ref in the watched ref set.




### luci.milo {#luci.milo}

```python
luci.milo(
    # Optional arguments.
    logo = None,
    favicon = None,
    monorail_project = None,
    monorail_components = None,
    bug_summary = None,
    bug_description = None,
)
```



Defines optional configuration of the Milo service for this project.

Milo service is a public user interface for displaying (among other things)
builds, builders, builder lists (see [luci.list_view(...)](#luci.list_view)) and consoles
(see [luci.console_view(...)](#luci.console_view)).

Can optionally be configured with a reference to a [Monorail] project to use
for filing bugs via custom feedback links on build pages. The format of a new
bug is defined via `bug_summary` and `bug_description` fields which are
interpreted as Golang [text templates]. They can either be given directly as
strings, or loaded from external files via [io.read_file(...)](#io.read_file).

Supported interpolations are the fields of the standard build proto such as:

    {{.Build.Builder.Project}}
    {{.Build.Builder.Bucket}}
    {{.Build.Builder.Builder}}

Other available fields include:

    {{.MiloBuildUrl}}
    {{.MiloBuilderUrl}}

If any specified placeholder cannot be satisfied then no URL is rendered for
the build page feedback link.

[Monorail]: https://bugs.chromium.org
[text templates]: https://golang.org/pkg/text/template

#### Arguments {#luci.milo-args}

* **logo**: optional https URL to the project logo, must be hosted on `storage.googleapis.com`.
* **favicon**: optional https URL to the project favicon, must be hosted on `storage.googleapis.com`.
* **monorail_project**: optional Monorail project to file bugs in when a user clicks the feedback link on a build page.
* **monorail_components**: a list of the Monorail component to assign to a new bug, in the hierarchical `>`-separated format, e.g. `Infra>Client>ChromeOS>CI`. Required if `monorail_project` is set, otherwise must not be used.
* **bug_summary**: string with a text template for generating new bug's summary given a builder on whose page a user clicked the feedback link. Must not be used if `monorail_project` is unset.
* **bug_description**: string with a text template for generating new bug's description given a builder on whose page a user clicked the feedback link. Must not be used if `monorail_project` is unset.




### luci.list_view {#luci.list_view}

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

Or separately one by one via [luci.list_view_entry(...)](#luci.list_view_entry) declarations:

    luci.list_view(name = 'Try builders')
    luci.list_view_entry(
        builder = 'win',
        list_view = 'Try builders',
    )
    luci.list_view_entry(
        builder = 'linux',
        list_view = 'Try builders',
    )

Note that declaring Buildbot builders (which is deprecated) requires the use
of [luci.list_view_entry(...)](#luci.list_view_entry). It's the only way to provide a reference to a
Buildbot builder (see `buildbot` field).

#### Arguments {#luci.list_view-args}

* **name**: a name of this view, will show up in URLs. Note that names of [luci.list_view(...)](#luci.list_view) and [luci.console_view(...)](#luci.console_view) are in the same namespace i.e. defining a list view with the same name as some console view (and vice versa) causes an error. Required.
* **title**: a title of this view, will show up in UI. Defaults to `name`.
* **favicon**: optional https URL to the favicon for this view, must be hosted on `storage.googleapis.com`. Defaults to `favicon` in [luci.milo(...)](#luci.milo).
* **entries**: a list of builders or [luci.list_view_entry(...)](#luci.list_view_entry) entities to include into this view.




### luci.list_view_entry {#luci.list_view_entry}

```python
luci.list_view_entry(builder = None, list_view = None, buildbot = None)
```



A builder entry in some [luci.list_view(...)](#luci.list_view).

Can be used to declare that a builder belongs to a list view outside of
the list view declaration. In particular useful in functions. For example:

    luci.list_view(name = 'Try builders')

    def try_builder(name, ...):
      luci.builder(name = name, ...)
      luci.list_view_entry(list_view = 'Try builders', builder = name)

Can also be used inline in [luci.list_view(...)](#luci.list_view) declarations, for consistency
with corresponding [luci.console_view_entry(...)](#luci.console_view_entry) usage. `list_view` argument
can be omitted in this case:

    luci.list_view(
        name = 'Try builders',
        entries = [
            luci.list_view_entry(builder = 'Win'),
            ...
        ],
    )

#### Arguments {#luci.list_view_entry-args}

* **builder**: a builder to add, see [luci.builder(...)](#luci.builder). Can be omitted for **extra deprecated** case of Buildbot-only views. `buildbot` field must be set in this case.
* **list_view**: a list view to add the builder to. Can be omitted if `list_view_entry` is used inline inside some [luci.list_view(...)](#luci.list_view) declaration.
* **buildbot**: a reference to an equivalent Buildbot builder, given as `<master>/<builder>` string. **Deprecated**. Exists only to aid in the migration off Buildbot.




### luci.console_view {#luci.console_view}

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
    entries = None,
)
```



A Milo UI view that displays a table-like console where columns are
builders and rows are git commits on which builders are triggered.

A console is associated with a single git repository it uses as a source of
commits to display as rows. The watched ref set is defined via `refs` and
optional `exclude_ref` fields. If `refs` are empty, the console defaults to
watching `refs/heads/master`.

`exclude_ref` is useful when watching for commits that landed specifically
on a branch. For example, the config below allows to track commits from all
release branches, but ignore the commits from the master branch, from which
these release branches are branched off:

    luci.console_view(
        ...
        refs = ['refs/branch-heads/\d+\.\d+'],
        exclude_ref = 'refs/heads/master',
        ...
    )

For best results, ensure commits on each watched ref have **committer**
timestamps monotonically non-decreasing. Gerrit will take care of this if you
require each commit to go through Gerrit by prohibiting "git push" on these
refs.

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

Or separately one by one via [luci.console_view_entry(...)](#luci.console_view_entry) declarations:

    luci.console_view(name = 'CI builders')
    luci.console_view_entry(
        builder = 'Windows Builder',
        console_view = 'CI builders',
        short_name = 'win',
        category = 'ci',
    )

#### Console headers

Consoles can have headers which are collections of links, oncall rotation
information, and console summaries that are displayed at the top of a console,
below the tree status information. Links and oncall information is always laid
out to the left, while console groups are laid out to the right. Each oncall
and links group take up a row.

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

[project.proto]: https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/master/milo/api/config/project.proto

#### Arguments {#luci.console_view-args}

* **name**: a name of this console, will show up in URLs. Note that names of [luci.console_view(...)](#luci.console_view) and [luci.list_view(...)](#luci.list_view) are in the same namespace i.e. defining a console view with the same name as some list view (and vice versa) causes an error. Required.
* **title**: a title of this console, will show up in UI. Defaults to `name`.
* **repo**: URL of a git repository whose commits are displayed as rows in the console. Must start with `https://`. Required.
* **refs**: a list of regular expressions that define the set of refs to pull commits from when displaying the console, e.g. `refs/heads/[^/]+` or `refs/branch-heads/\d+\.\d+`. The regular expression should have a literal prefix with at least two slashes present, e.g. `refs/release-\d+/foobar` is *not allowed*, because the literal prefix `refs/release-` contains only one slash. The regexp should not start with `^` or end with `$` as they will be added automatically.  If empty, defaults to `['refs/heads/master']`.
* **exclude_ref**: a single ref, commits from which are ignored even when they are reachable from refs specified via `refs` and `refs_regexps`. Note that force pushes to this ref are not supported. Milo uses caching assuming set of commits reachable from this ref may only grow, never lose some commits.
* **header**: either a string with a path to the file with the header definition (see [io.read_file(...)](#io.read_file) for the acceptable path format), or a dict with the header definition.
* **include_experimental_builds**: if True, this console will not filter out builds marked as Experimental. By default consoles only show production builds.
* **favicon**: optional https URL to the favicon for this console, must be hosted on `storage.googleapis.com`. Defaults to `favicon` in [luci.milo(...)](#luci.milo).
* **entries**: a list of [luci.console_view_entry(...)](#luci.console_view_entry) entities specifying builders to show on the console.




### luci.console_view_entry {#luci.console_view_entry}

```python
luci.console_view_entry(
    # Optional arguments.
    builder = None,
    short_name = None,
    category = None,
    console_view = None,
    buildbot = None,
)
```



A builder entry in some [luci.console_view(...)](#luci.console_view).

Used inline in [luci.console_view(...)](#luci.console_view) declarations to provide `category` and
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

#### Arguments {#luci.console_view_entry-args}

* **builder**: a builder to add, see [luci.builder(...)](#luci.builder). Can be omitted for **extra deprecated** case of Buildbot-only views. `buildbot` field must be set in this case.
* **short_name**: a string with the 1-3 character abbreviation of the builder.
* **category**: a string of the form `term1|term2|...` that describes the hierarchy of the builder columns. Neighboring builders with common ancestors will have their column headers merged.
* **console_view**: a console view to add the builder to. Can be omitted if `console_view_entry` is used inline inside some [luci.console_view(...)](#luci.console_view) declaration.
* **buildbot**: a reference to an equivalent Buildbot builder, given as `<master>/<builder>` string. **Deprecated**. Exists only to aid in the migration off Buildbot.




### luci.cq {#luci.cq}

```python
luci.cq(
    # Optional arguments.
    submit_max_burst = None,
    submit_burst_delay = None,
    draining_start_time = None,
    status_host = None,
)
```



Defines optional configuration of the CQ service for this project.

CQ is a service that monitors Gerrit CLs in a configured set of Gerrit
projects, launches presubmit jobs (aka tryjobs) whenever a CL is marked as
ready for CQ, and submits the CL if it passes all checks.

This optional rule can be used to set global CQ parameters that apply to all
[luci.cq_group(...)](#luci.cq_group) defined in the project.

#### Arguments {#luci.cq-args}

* **submit_max_burst**: maximum number of successful CQ attempts completed by submitting corresponding Gerrit CL(s) before waiting `submit_burst_delay`. This feature today applies to all attempts processed by CQ, across all [luci.cq_group(...)](#luci.cq_group) instances. Optional, by default there's no limit. If used, requires `submit_burst_delay` to be set too.
* **submit_burst_delay**: how long to wait between bursts of submissions of CQ attempts. Required if `submit_max_burst` is used.
* **draining_start_time**: if present, the CQ will refrain from processing any CLs, on which CQ was triggered after the specified time. This is an UTC RFC3339 string representing the time, e.g. `2017-12-23T15:47:58Z` and Z is mandatory.
* **status_host**: hostname of the CQ status app to push updates to. Optional and deprecated.




### luci.cq_group {#luci.cq_group}

```python
luci.cq_group(
    # Required arguments.
    name,
    watch,

    # Optional arguments.
    acls = None,
    allow_submit_with_open_deps = None,
    tree_status_host = None,
    retry_config = None,
    verifiers = None,
)
```



Defines a set of refs to be watched by the CQ and a set of verifiers to run
whenever there's a pending approved CL for a ref in the watched set.

#### Arguments {#luci.cq_group-args}

* **name**: a name of this CQ group, to reference it in other rules. Doesn't show up anywhere in configs or UI. Required.
* **watch**: either a single [cq.refset(...)](#cq.refset) or a list of [cq.refset(...)](#cq.refset) (one per repo), defining what set of refs the CQ should monitor for pending CLs. Required.
* **acls**: list of [acl.entry(...)](#acl.entry) objects with ACLs specific for this CQ group. Only `acl.CQ_*` roles are allowed here. By default ACLs are inherited from [luci.project(...)](#luci.project) definition. At least one `acl.CQ_COMMITTER` entry should be provided somewhere (either here or in [luci.project(...)](#luci.project)).
* **allow_submit_with_open_deps**: controls how a CQ full run behaves when the current Gerrit CL has open dependencies (not yet submitted CLs on which *this* CL depends). If set to False (default), the CQ will abort a full run attempt immediately if open dependencies are detected. If set to True, then the CQ will not abort a full run, and upon passing all other verifiers, the CQ will attempt to submit the CL regardless of open dependencies and whether the CQ verified those open dependencies. In turn, if the Gerrit project config allows this, Gerrit will submit all dependent CLs first and then this CL.
* **tree_status_host**: a hostname of the project tree status app (if any). It is used by the CQ to check the tree status before committing a CL. If the tree is closed, then the CQ will wait until it is reopened.
* **retry_config**: a new [cq.retry_config(...)](#cq.retry_config) struct or one of `cq.RETRY_*` constants that define how CQ should retry failed builds. See [CQ](#cq_doc) for more info. Default is `cq.RETRY_TRANSIENT_FAILURES`.
* **verifiers**: a list of [luci.cq_tryjob_verifier(...)](#luci.cq_tryjob_verifier) specifying what checks to run on a pending CL. See [luci.cq_tryjob_verifier(...)](#luci.cq_tryjob_verifier) for all details. As a shortcut, each entry can also either be a dict or a string. A dict entry is an alias for `luci.cq_tryjob_verifier(**entry)` and a string entry is an alias for `luci.cq_tryjob_verifier(builder = entry)`.




### luci.cq_tryjob_verifier {#luci.cq_tryjob_verifier}

```python
luci.cq_tryjob_verifier(
    # Required arguments.
    builder,

    # Optional arguments.
    cq_group = None,
    disable_reuse = None,
    experiment_percentage = None,
    location_regexp = None,
    location_regexp_exclude = None,
)
```



A verifier in a [luci.cq_group(...)](#luci.cq_group) that triggers tryjobs for verifying CLs.

When processing a CL, the CQ examines a list of registered verifiers and
launches new corresponding builds (called "tryjobs") if it decides this is
necessary (per the configuration of the verifier and the previous history
of this CL).

The CQ automatically retries failed tryjobs (per configured `retry_config` in
[luci.cq_group(...)](#luci.cq_group)) and only allows CL to land if each builder has succeeded
in the latest retry. If a given tryjob result is too old (>1 day) it is
ignored.

#### Filtering based on files touched by a CL

The CQ can examine a set of files touched by the CL and decide to skip this
verifier. Touching a file means either adding, modifying or removing it.

This is controlled by `location_regexp` and `location_regexp_exclude` fields:

  * If `location_regexp` is specified and no file in a CL matches any of the
    `location_regexp`, then the CQ will not care about this verifier.
  * If a file in a CL matches any `location_regexp_exclude`, then this file
    won't be considered when matching `location_regexp`.
  * If `location_regexp_exclude` is specified, but `location_regexp` is not,
    `location_regexp` is implied to be `.*`.
  * If neither `location_regexp` nor `location_regexp_exclude` are specified
    (default), the verifier will be used on all CLs.

The matches are done against the following string:

    <gerrit_url>/<gerrit_project_name>/+/<cl_file_path>

The file path is relative to the repo root, and it uses Unix `/` directory
separator.

The comparison is a full match. The pattern is implicitly anchored with `^`
and `$`, so there is no need add them.

This filtering currently cannot be used in any of the following cases:

  * For experimental verifiers (when `experiment_percentage` is non-zero).
  * For verifiers in CQ groups with `allow_submit_with_open_deps = True`.

Please talk to CQ owners if these restrictions are limiting you.

##### Examples

Enable the verifier for all CLs touching any file in `third_party/WebKit`
directory of the `chromium/src` repo, but not directory itself:

    luci.cq_tryjob_verifier(
        location_regexp = [
            'https://chromium-review.googlesource.com/chromium/src/[+]/third_party/WebKit/.+',
        ],
    )

Match a CL which touches at least one file other than `one.txt` inside `all/`
directory of the Gerrit project `repo`:

    luci.cq_tryjob_verifier(
        location_regexp = ['https://example.com/repo/[+]/.+'],
        location_regexp_exclude = ['https://example.com/repo/[+]/all/one.txt'],
    )

Match a CL which touches at least one file other than `one.txt` in any
repository **or** belongs to any other Gerrit server. Note, in this case
`location_regexp` defaults to `.*`:

    luci.cq_tryjob_verifier(
        location_regexp_exclude = ['https://example.com/repo/[+]/all/one.txt'],
    )

#### Declaring verifiers

`cq_tryjob_verifier` is used inline in [luci.cq_group(...)](#luci.cq_group) declarations to
provide per-builder verifier parameters. `cq_group` argument can be omitted in
this case:

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


It can also be associated with a [luci.cq_group(...)](#luci.cq_group) outside of
[luci.cq_group(...)](#luci.cq_group) declaration. This is in particular useful in functions.
For example:

    luci.cq_group(name = 'Main CQ')

    def try_builder(name, ...):
      luci.builder(name = name, ...)
      luci.cq_tryjob_verifier(builder = name, cq_group = 'Main CQ')

#### Arguments {#luci.cq_tryjob_verifier-args}

* **builder**: a builder to launch when verifying a CL, see [luci.builder(...)](#luci.builder). As an **experimental** and **unstable** extension, can also be a string that starts with `external/`. It indicates that this builder is defined elsewhere, and its name should be passed to the CQ as is. In particular, to reference a builder in another LUCI project, use `external/<project>/<bucket>/<builder>` (filling in concrete values). To reference a Buildbot builder use `external/*/<master>/<builder>`, e.g. `external/*/master.tryserver.chromium.android/android_tester`. Required.
* **cq_group**: a CQ group to add the verifier to. Can be omitted if `cq_tryjob_verifier` is used inline inside some [luci.cq_group(...)](#luci.cq_group) declaration.
* **disable_reuse**: if True, a fresh build will be required for each CQ attempt. Default is False, meaning the CQ may re-use a successful build triggered before the current CQ attempt started. This option is typically used for verifiers which run presubmit scripts, which are supposed to be quick to run and provide additional OWNERS, lint, etc. checks which are useful to run against the latest revision of the CL's target branch.
* **experiment_percentage**: when this field is present, it marks the verifier as experimental. Such verifier is only triggered on a given percentage of the CLs and the outcome does not affect the decicion whether a CL can land or not. This is typically used to test new builders and estimate their capacity requirements.
* **location_regexp**: a list of regexps that define a set of files whose modification trigger this verifier. See the explanation above for all details.
* **location_regexp_exclude**: a list of regexps that define a set of files to completely skip when evaluating whether the verifier should be applied to a CL or not. See the explanation above for all details.






## ACLs

### Roles {#roles_doc}

Below is the table with role constants that can be passed as `roles` in
[acl.entry(...)](#acl.entry).

Due to some inconsistencies in how LUCI service are currently implemented, some
roles can be assigned only in [luci.project(...)](#luci.project) rule, but
some also in individual [luci.bucket(...)](#luci.bucket) or
[luci.cq_group(...)](#luci.cq_group) rules.

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





### acl.entry {#acl.entry}

```python
acl.entry(roles, groups = None, users = None)
```



Returns an ACL binding which assigns given role (or roles) to given
individuals or groups.

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


#### Returns  {#acl.entry-returns}

acl.entry object, should be treated as opaque.





## Swarming




### swarming.cache {#swarming.cache}

```python
swarming.cache(path, name = None, wait_for_warm_cache = None)
```



Represents a request for the bot to mount a named cache to a path.

Each bot has a LRU of named caches: think of them as local named directories
in some protected place that survive between builds.

A build can request one or more such caches to be mounted (in read/write mode)
at the requested path relative to some known root. In recipes-based builds,
the path is relative to `api.paths['cache']` dir.

If it's the first time a cache is mounted on this particular bot, it will
appear as an empty directory. Otherwise it will contain whatever was left
there by the previous build that mounted exact same named cache on this bot,
even if that build is completely irrelevant to the current build and just
happened to use the same named cache (sometimes this is useful to share state
between different builders).

At the end of the build the cache directory is unmounted. If at that time the
bot is running out of space, caches (in their entirety, the named cache
directory and all files inside) are evicted in LRU manner until there's enough
free disk space left. Renaming a cache is equivalent to clearing it from the
builder perspective. The files will still be there, but eventually will be
purged by GC.

Additionally, Buildbucket always implicitly requests to mount a special
builder cache to 'builder' path:

    swarming.cache('builder', name=some_hash('<project>/<bucket>/<builder>'))

This means that any LUCI builder has a "personal disk space" on the bot.
Builder cache is often a good start before customizing caching. In recipes, it
is available at `api.path['cache'].join('builder')`.

In order to share the builder cache directory among multiple builders, some
explicitly named cache can be mounted to `builder` path on these builders.
Buildbucket will not try to override it with its auto-generated builder cache.

For example, if builders **A** and **B** both declare they use named cache
`swarming.cache('builder', name='my_shared_cache')`, and an **A** build ran on
a bot and left some files in the builder cache, then when a **B** build runs
on the same bot, the same files will be available in its builder cache.

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

Intended to be used as a value in `dimensions` dict of [luci.builder(...)](#luci.builder) when
using dimensions that expire:

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



### swarming.validate_caches {#swarming.validate_caches}

```python
swarming.validate_caches(attr, caches)
```


*** note
**Advanced function.** It is not used for common use cases.
***


Validates a list of caches.

Ensures each entry is swarming.cache struct, and no two entries use same
name or path.

#### Arguments {#swarming.validate_caches-args}

* **attr**: field name with caches, for error messages. Required.
* **caches**: a list of [swarming.cache(...)](#swarming.cache) entries to validate. Required.


#### Returns  {#swarming.validate_caches-returns}

Validates list of caches (may be an empty list, never None).



### swarming.validate_dimensions {#swarming.validate_dimensions}

```python
swarming.validate_dimensions(attr, dimensions)
```


*** note
**Advanced function.** It is not used for common use cases.
***


Validates and normalizes a dict with dimensions.

The dict should have string keys and values are swarming.dimension, a string
or a list of thereof (for repeated dimensions).

#### Arguments {#swarming.validate_dimensions-args}

* **attr**: field name with dimensions, for error messages. Required.
* **dimensions**: a dict `{string: string|swarming.dimension}`. Required.


#### Returns  {#swarming.validate_dimensions-returns}

Validated and normalized dict in form `{string: [swarming.dimension]}`.



### swarming.validate_tags {#swarming.validate_tags}

```python
swarming.validate_tags(attr, tags)
```


*** note
**Advanced function.** It is not used for common use cases.
***


Validates a list of `k:v` pairs with Swarming tags.

#### Arguments {#swarming.validate_tags-args}

* **attr**: field name with tags, for error messages. Required.
* **tags**: a list of tags to validate. Required.


#### Returns  {#swarming.validate_tags-returns}

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
)
```



Policy for how LUCI Scheduler should handle incoming triggering requests.

This policy defines when and how LUCI Scheduler should launch new builds in
response to triggering requests from [luci.gitiles_poller(...)](#luci.gitiles_poller) or from
EmitTriggers RPC call.

The following batching strategies are supported:

  * `scheduler.GREEDY_BATCHING_KIND`: use a greedy batching function that
    takes all pending triggering requests (up to `max_batch_size` limit) and
    collapses them into one new build. It doesn't wait for a full batch, nor
    tries to batch evenly.
  * `scheduler.LOGARITHMIC_BATCHING_KIND`: use a logarithmic batching function
    that takes log(N) pending triggers (up to `max_batch_size` limit) and
    collapses them into one new build, where N is the total number of pending
    triggers. The base of the logarithm is defined by `log_base`.

#### Arguments {#scheduler.policy-args}

* **kind**: one of `*_BATCHING_KIND` values above. Required.
* **max_concurrent_invocations**: limit on a number of builds running at the same time. If the number of currently running builds launched through LUCI Scheduler is more than or equal to this setting, LUCI Scheduler will keep queuing up triggering requests, waiting for some running build to finish before starting a new one. Default is 1.
* **max_batch_size**: limit on how many pending triggering requests to "collapse" into a new single build. For example, setting this to 1 will make each triggering request result in a separate build. When multiple triggering request are collapsed into a single build, properties of the most recent triggering request are used to derive properties for the build. For example, when triggering requests come from a [luci.gitiles_poller(...)](#luci.gitiles_poller), only a git revision from the latest triggering request (i.e. the latest commit) will end up in the build properties. Default is 1000 (effectively unlimited).
* **log_base**: base of the logarithm operation during logarithmic batching. For example, setting this to 2, will cause 3 out of 8 pending triggering requests to be combined into a single build. Required when using `LOGARITHMIC_BATCHING_KIND`, ignored otherwise. Must be larger or equal to 1.0001 for numerical stability reasons.


#### Returns  {#scheduler.policy-returns}

An opaque triggering policy object.



### scheduler.greedy_batching {#scheduler.greedy_batching}

```python
scheduler.greedy_batching(max_concurrent_invocations = None, max_batch_size = None)
```



A shortcut for `scheduler.policy(scheduler.GREEDY_BATCHING_KIND, ...).`

See [scheduler.policy(...)](#scheduler.policy) for all details.

#### Arguments {#scheduler.greedy_batching-args}

* **max_concurrent_invocations**: see [scheduler.policy(...)](#scheduler.policy).
* **max_batch_size**: see [scheduler.policy(...)](#scheduler.policy).




### scheduler.logarithmic_batching {#scheduler.logarithmic_batching}

```python
scheduler.logarithmic_batching(log_base, max_concurrent_invocations = None, max_batch_size = None)
```



A shortcut for `scheduler.policy(scheduler.LOGARITHMIC_BATCHING_KIND, ...)`.

See [scheduler.policy(...)](#scheduler.policy) for all details.

#### Arguments {#scheduler.logarithmic_batching-args}

* **log_base**: see [scheduler.policy(...)](#scheduler.policy). Required.
* **max_concurrent_invocations**: see [scheduler.policy(...)](#scheduler.policy).
* **max_batch_size**: see [scheduler.policy(...)](#scheduler.policy).






## CQ  {#cq_doc}

CQ module exposes structs and enums useful when defining [luci.cq_group(...)](#luci.cq_group)
entities.

`cq.RETRY_*` constants define some commonly used values for `retry_config`
field of [luci.cq_group(...)](#luci.cq_group):

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


### cq.refset {#cq.refset}

```python
cq.refset(repo, refs = None)
```



Defines a repository and a subset of its refs.

Used in `watch` field of [luci.cq_group(...)](#luci.cq_group) to specify what refs the CQ should
be monitoring.

*** note
**Note:** Gerrit ACLs must be configured such that the CQ has read access to
these refs, otherwise users will be waiting for the CQ to act on their CLs
forever.
***

#### Arguments {#cq.refset-args}

* **repo**: URL of a git repository to watch, starting with `https://`. Only repositories hosted on `*.googlesource.com` are supported currently. Required.
* **refs**: a list of regular expressions that define the set of refs to watch for CLs, e.g. `refs/heads/.+`. If not set, defaults to `refs/heads/master`.


#### Returns  {#cq.refset-returns}

An opaque struct to be passed to `watch` field of [luci.cq_group(...)](#luci.cq_group).



### cq.retry_config {#cq.retry_config}

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
can be passed as `retry_config` field to [luci.cq_group(...)](#luci.cq_group).

Some commonly used presents are available as `cq.RETRY_*` constants. See
[CQ](#cq_doc) for more info.

#### Arguments {#cq.retry_config-args}

* **single_quota**: retry quota for a single tryjob.
* **global_quota**: retry quota for all tryjobs in a CL.
* **failure_weight**: the weight assigned to each tryjob failure.
* **transient_failure_weight**: the weight assigned to each transient (aka "infra") failure.
* **timeout_weight**: weight assigned to tryjob timeouts.


#### Returns  {#cq.retry_config-returns}

cq.retry_config struct.





## Built-in constants and functions

Refer to the list of [built-in constants and functions][starlark-builtins]
exposed in the global namespace by Starlark itself.

[starlark-builtins]: https://github.com/google/starlark-go/blob/master/doc/spec.md#built-in-constants-and-functions

In addition, `lucicfg` exposes the following functions.





### __load {#__load}

```python
__load(module)
```



Loads another Starlark module (if it haven't been loaded before), extracts
one or more values from it, and binds them to names in the current module.

This is not actually a function, but a core Starlark statement. We give it
additional meaning (related to [the execution model](#execution_doc)) worthy
of additional documentation.

A load statement requires at least two "arguments". The first must be a
literal string, it identifies the module to load. The remaining arguments are
a mixture of literal strings, such as `'x'`, or named literal strings, such as
`y='x'`.

The literal string (`'x'`), which must denote a valid identifier not starting
with `_`, specifies the name to extract from the loaded module. In effect,
names starting with `_` are not exported. The name (`y`) specifies the local
name. If no name is given, the local name matches the quoted name.

```
load('//module.star', 'x', 'y', 'z')       # assigns x, y, and z
load('//module.star', 'x', y2='y', 'z')    # assigns x, y2, and z
```

A load statement within a function is a static error.

TODO(vadimsh): Write about 'load' and 'exec' contexts and how 'load' and
'exec' interact with each other.

#### Arguments {#__load-args}

* **module**: module to load, i.e. `//path/within/current/package.star` or `@<pkg>//path/within/pkg.star`. Required.




### exec {#exec}

```python
exec(module)
```



Executes another Starlark module for its side effects.

TODO(vadimsh): Write about 'load' and 'exec' contexts and how 'load' and
'exec' interact with each other.

#### Arguments {#exec-args}

* **module**: module to execute, i.e. `//path/within/current/package.star` or `@<pkg>//path/within/pkg.star`. Required.


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



Returns an immutable struct object with fields populated from the specified
keyword arguments.

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




### to_json {#to_json}

```python
to_json(value)
```



Serializes a value to a compact JSON string.

Doesn't support integers that do not fit int64. Fails if the value has cycles.

#### Arguments {#to_json-args}

* **value**: a primitive Starlark value: a scalar, or a list/tuple/dict containing only primitive Starlark values. Required.









### proto.to_textpb {#proto.to_textpb}

```python
proto.to_textpb(msg)
```



Serializes a protobuf message to a string using ASCII proto serialization.

#### Arguments {#proto.to_textpb-args}

* **msg**: a proto message to serialize. Required.




### proto.to_jsonpb {#proto.to_jsonpb}

```python
proto.to_jsonpb(msg, emit_defaults = None)
```



Serializes a protobuf message to a string using JSONPB serialization.

#### Arguments {#proto.to_jsonpb-args}

* **msg**: a proto message to serialize. Required.
* **emit_defaults**: if True, do not omit fields with default values.




### proto.from_textpb {#proto.from_textpb}

```python
proto.from_textpb(ctor, text)
```



Deserializes a protobuf message given its ASCII proto serialization.

#### Arguments {#proto.from_textpb-args}

* **ctor**: a message constructor function, same one you would normally use to create a new message. Required.
* **text**: a string with the serialized message. Required.


#### Returns  {#proto.from_textpb-returns}

Deserialized message constructed via `ctor`.



### proto.from_jsonpb {#proto.from_jsonpb}

```python
proto.from_jsonpb(ctor, text)
```



Deserializes a protobuf message given its JSONPB serialization.

#### Arguments {#proto.from_jsonpb-args}

* **ctor**: a message constructor function, same one you would normally use to create a new message. Required.
* **text**: a string with the serialized message. Required.


#### Returns  {#proto.from_jsonpb-returns}

Deserialized message constructed via `ctor`.








### io.read_file {#io.read_file}

```python
io.read_file(path)
```



Reads a file and returns its contents as a string.

Useful for rules that accept large chunks of free form text. By using
`io.read_file` such text can be kept in a separate file.

#### Arguments {#io.read_file-args}

* **path**: either a path relative to the currently executing Starlark script, or (if starts with `//`) an absolute path within the currently executing package. If it is a relative path, it must point somewhere inside the current package directory. Required.


#### Returns  {#io.read_file-returns}

The contents of the file as a string. Fails if there's no such file, it
can't be read, or it is outside of the current package directory.



### io.read_proto {#io.read_proto}

```python
io.read_proto(ctor, path, encoding = None)
```



Reads a serialized proto message from a file, deserializes and returns it.

#### Arguments {#io.read_proto-args}

* **ctor**: a constructor function that defines the message type. Required.
* **path**: either a path relative to the currently executing Starlark script, or (if starts with `//`) an absolute path within the currently executing package. If it is a relative path, it must point somewhere inside the current package directory. Required.
* **encoding**: either `jsonpb` or `textpb` or `auto` to detect based on the file extension. Default is `auto`.


#### Returns  {#io.read_proto-returns}

Deserialized proto message constructed via `ctor`.




