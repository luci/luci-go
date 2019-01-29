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


### Resolving naming ambiguities

*** note
TODO: To be written.
***


### Defining cron schedules {#schedules_doc}

*** note
TODO: To be written.
***


## Configuring and inspecting lucicfg itself




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
* **config_set**: name of the config set in LUCI Config Service to use for validation. Default is `projects/<name>` where `<name>` is taken from [core.project(...)](#core.project) rule. If there's no such rule, the default is "", meaning the generated config will not be validated via LUCI Config Service.
* **config_dir**: a directory to place generated configs into, relative to the directory that contains the entry point \*.star file. `..` is allowed. If set via `-config-dir` command line flag, it is relative to the current working directory. Will be created if absent. If `-`, the configs are just printed to stdout in a format useful for debugging. Default is "generated".
* **tracked_files**: a list of glob patterns that define a subset of files under `config_dir` that are considered generated. This is important if some generated file disappears from `lucicfg` output: it must be deleted from the disk as well. To do this, `lucicfg` needs to know what files are safe to delete. Each entry is either `<glob pattern>` (a "positive" glob) or `!<glob pattern>` (a "negative" glob). A file under `config_dir` (or any of its subdirectories) is considered tracked if it matches any of the positive globs and none of the negative globs. For example, `tracked_files` for prod and dev projects co-hosted in the same directory may look like `['*.cfg', '!*-dev.cfg']` for prod and `['*-dev.cfg']` for dev. If `tracked_files` is empty (default), lucicfg will never delete any files. In this case it is responsibility of the caller to make sure no stale output remains.
* **fail_on_warnings**: if set to True treat validation warnings as errors. Default is False (i.e. warnings do to cause the validation to fail). If set to True via `lucicfg.config` and you want to override it to False via command line flags use `-fail-on-warnings=false`.




### lucicfg.version {#lucicfg.version}

```python
lucicfg.version()
```



Returns a triple with lucicfg version: `(major, minor, revision)`.





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





## Core rules




### core.project {#core.project}

```python
core.project(
    # Required arguments.
    name,

    # Optional arguments.
    buildbucket = None,
    logdog = None,
    scheduler = None,
    swarming = None,
    acls = None,
)
```



Defines a LUCI project.

There should be exactly one such definition in the top-level config file.

#### Arguments {#core.project-args}

* **name**: full name of the project. Required.
* **buildbucket**: hostname of a Buildbucket service to use (if any).
* **logdog**: hostname of a LogDog service to use (if any).
* **scheduler**: hostname of a LUCI Scheduler service to use (if any).
* **swarming**: hostname of a Swarming service to use (if any).
* **acls**: list of [acl.entry(...)](#acl.entry) objects, will be inherited by all buckets.




### core.logdog {#core.logdog}

```python
core.logdog(gs_bucket = None)
```



Defines configuration of the LogDog service for this project.

Usually required for any non-trivial project.

#### Arguments {#core.logdog-args}

* **gs_bucket**: base Google Storage archival path, archive logs will be written to this bucket/path.




### core.bucket {#core.bucket}

```python
core.bucket(name, acls = None)
```



Defines a bucket: a container for LUCI resources that share the same ACL.

#### Arguments {#core.bucket-args}

* **name**: name of the bucket, e.g. `ci` or `try`. Required.
* **acls**: list of [acl.entry(...)](#acl.entry) objects.




### core.recipe {#core.recipe}

```python
core.recipe(
    # Required arguments.
    name,
    cipd_package,

    # Optional arguments.
    cipd_version = None,
    recipe = None,
)
```



Defines where to locate a particular recipe.

Builders refer to recipes in their `recipe` field, see [core.builder(...)](#core.builder).
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

#### Arguments {#core.recipe-args}

* **name**: name of this recipe entity, to refer to it from builders. If `recipe` is None, also specifies the recipe name within the bundle. Required.
* **cipd_package**: a cipd package name with the recipe bundle. Required.
* **cipd_version**: a version of the recipe bundle package to fetch, default is `refs/heads/master`.
* **recipe**: name of a recipe inside the recipe bundle if it differs from `name`. Useful if recipe names clash between different recipe bundles. When this happens, `name` can be used as a non-ambiguous alias, and `recipe` can provide the actual recipe name. Defaults to `name`.




### core.builder {#core.builder}

```python
core.builder(
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
core.bucket(
    ...
    acls = [
        ...
        acl.entry(acl.BUILDBUCKET_TRIGGERER, <builder service account>),
        ...
    ],
)
```

This is not necessary if the recipe uses Scheduler API instead of Buildbucket.

#### Arguments {#core.builder-args}

* **name**: name of the builder, will show up in UIs and logs. Required.
* **bucket**: name of the bucket the builder belongs to. Required.
* **recipe**: name of a recipe to run, see [core.recipe(...)](#core.recipe) rule. Required.
* **properties**: a dict with string keys and JSON-serializable values, defining properties to pass to the recipe.
* **service_account**: an email of a service account to run the recipe under: the recipe (and various tools it calls, e.g. gsutil) will be able to make outbound HTTP calls that have an OAuth access token belonging to this service account (provided it is registered with LUCI).
* **caches**: a list of [swarming.cache(...)](#swarming.cache) objects describing Swarming named caches that should be present on the bot. See [swarming.cache(...)](#swarming.cache) doc for more details.
* **execution_timeout**: how long to wait for a running build to finish before forcefully aborting it and marking the build as timed out. If None, defer the decision to Buildbucket service.
* **dimensions**: a dict with swarming dimensions, indicating requirements for a bot to execute the build. Keys are strings (e.g. `os`), and values are either strings (e.g. `Linux`), [swarming.dimension(...)](#swarming.dimension) objects (for defining expiring dimensions) or lists of thereof.
* **priority**: int [1-255] or None, indicating swarming task priority, lower is more important. If None, defer the decision to Buildbucket service.
* **swarming_tags**: a list of tags (`k:v` strings) to assign to the Swarming task that runs the builder. Each tag will also end up in `swarming_tag` Buildbucket tag, for example `swarming_tag:builder:release`.
* **expiration_timeout**: how long to wait for a build to be picked up by a matching bot (based on `dimensions`) before canceling the build and marking it as expired. If None, defer the decision to Buildbucket service.
* **schedule**: string with a cron schedule that describes when to run this builder. See [Defining cron schedules](#schedules_doc) for the expected format of this field. If None, the builder will not be running periodically.
* **triggering_policy**: [scheduler.policy(...)](#scheduler.policy) struct with a configuration that defines when and how LUCI Scheduler should launch new builds in response to triggering requests from [core.gitiles_poller(...)](#core.gitiles_poller) or from EmitTriggers API. Does not apply to builds started directly through Buildbucket. By default, only one concurrent build is allowed and while it runs, triggering requests accumulate in a queue. Once the build finishes, if the queue is not empty, a new build starts right away, "consuming" all pending requests. See [scheduler.policy(...)](#scheduler.policy) doc for more details.
* **build_numbers**: if True, generate monotonically increasing contiguous numbers for each build, unique within the builder. If None, defer the decision to Buildbucket service.
* **experimental**: if True, by default a new build in this builder will be marked as experimental. This is seen from recipes and they may behave differently (e.g. avoiding any side-effects). If None, defer the decision to Buildbucket service.
* **task_template_canary_percentage**: int [0-100] or None, indicating percentage of builds that should use a canary swarming task template. If None, defer the decision to Buildbucket service.
* **luci_migration_host**: deprecated setting that was important during the migration from Buildbot to LUCI. Refer to Buildbucket docs for the meaning.
* **triggers**: names of builders this builder triggers.
* **triggered_by**: names of builders or pollers this builder is triggered by.




### core.gitiles_poller {#core.gitiles_poller}

```python
core.gitiles_poller(
    # Required arguments.
    name,
    bucket,
    repo,

    # Optional arguments.
    refs = None,
    refs_regexps = None,
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
    test the latest commit. See [core.builder(...)](#core.builder) and [scheduler.policy(...)](#scheduler.policy)
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

The watched ref set is defined via `refs` and `refs_regexps` fields. One is
just a simple enumeration of refs, and another allows to use regular
expressions to define what refs belong to the watched set. Both fields can
be used at the same time. If neither is set, the gitiles_poller defaults to
watching `refs/heads/master`.

#### Arguments {#core.gitiles_poller-args}

* **name**: name of the poller, to refer to it from other rules. Required.
* **bucket**: name of the bucket the poller belongs to. Required.
* **repo**: URL of a git repository to poll, starting with `https://`. Required.
* **refs**: a list of fully qualified refs to watch, e.g. `refs/heads/master` or `refs/tags/v1.2.3`.
* **refs_regexps**: a list of regular expressions that define the watched set of refs, e.g. `refs/heads/[^/]+` or `refs/branch-heads/\d+\.\d+`. The regular expression should have a literal prefix with at least two slashes present, e.g. `refs/release-\d+/foobar` is *not allowed*, because the literal prefix `refs/release-` contains only one slash. The regexp should not start with `^` or end with `$` as they will be added automatically.
* **schedule**: string with a schedule that describes when to run one iteration of the poller. See [Defining cron schedules](#schedules_doc) for the expected format of this field. Note that it is rare to use custom schedules for pollers. By default, the poller will run each 30 sec.
* **triggers**: names of builders to trigger whenever the poller detects a new git commit on any ref in the watched ref set.






## ACLs

### Roles {#roles_doc}

Below is the table with role constants that can be passed as `roles` in
[acl.entry(...)](#acl.entry).

Due to some inconsistencies in how LUCI service are currently implemented, some
roles can be assigned only in [core.project(...)](#core.project) rule, but
some also in individual [core.bucket(...)](#core.bucket) rules.

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





### acl.entry {#acl.entry}

```python
acl.entry(roles, groups = None, users = None)
```



Returns an ACL binding which assigns given role (or roles) to given
individuals or groups.

Lists of acl.entry structs are passed to `acls` fields of [core.project(...)](#core.project)
and [core.bucket(...)](#core.bucket) rules.

An empty ACL binding is allowed. It is ignored everywhere. Useful for things
like:

```python
core.project(
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

Intended to be used as a value in `dimensions` dict of [core.builder(...)](#core.builder) when
using dimensions that expire:

```python
core.builder(
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
response to triggering requests from [core.gitiles_poller(...)](#core.gitiles_poller) or from
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
* **max_batch_size**: limit on how many pending triggering requests to "collapse" into a new single build. For example, setting this to 1 will make each triggering request result in a separate build. When multiple triggering request are collapsed into a single build, properties of the most recent triggering request are used to derive properties for the build. For example, when triggering requests come from a [core.gitiles_poller(...)](#core.gitiles_poller), only a git revision from the latest triggering request (i.e. the latest commit) will end up in the build properties. Default is 1000 (effectively unlimited).
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






## Built-in constants and functions

Refer to the list of [built-in constants and functions][starlark-builtins]
exposed in the global namespace by Starlark itself.

[starlark-builtins]: https://github.com/google/starlark-go/blob/master/doc/spec.md#built-in-constants-and-functions

In addition, `lucicfg` exposes the following functions.





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









### proto.to_pbtext {#proto.to_pbtext}

```python
proto.to_pbtext(msg)
```



Serializes a protobuf message to a string using ASCII proto serialization.

#### Arguments {#proto.to_pbtext-args}

* **msg**: a proto message to serialize. Required.




### proto.to_jsonpb {#proto.to_jsonpb}

```python
proto.to_jsonpb(msg)
```



Serializes a protobuf message to a string using JSONPB serialization.

#### Arguments {#proto.to_jsonpb-args}

* **msg**: a proto message to serialize. Required.





