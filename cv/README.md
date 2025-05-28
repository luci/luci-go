# LUCI Change Verifier

LUCI Change Verifier (CV) is the LUCI microservice that is responsible for
running pre-submit tests and submitting CLs when they pass all checks.

## What's here?

 - [api](./api): Protobuf files specifying the public API of CV,
   such as per-project config spec, PubSub messages and RPCs definitions.
   APIs organization is a bit messy right now, but of note are 2 versions:
     - [v0](./api/v0): experimental API under development. Request an approval
       from CV owners before using it.
     - [v1](./api/v1): stable API, which won't be broken unless absolutely
       necessary. As of Sep 2021, it's under the development.
 - [appengine](./appengine): the entry point for a GAE app.
     - [appengine/templates](./appengine/templates): HTML templates for
       debug web-based UI of CV. These are rendered by
       [internal/userhtml](./internal/userhtml) Go package.
     - [appengine/static](./appengine/static): static resources loaded by web UI
       directly.
 - [internal](./internal): GAE-agnostic implementation details in Go. See
   `godoc`-based overview
   [here](https://pkg.go.dev/go.chromium.org/luci/cv/internal). Notably:
     - [internal/cvtesting/e2e](./internal/cvtesting/e2e) high level end-to-end
       level CV tests, which cover the most important business logic in a fairly
       readable and concise way. The test are run against a [fake
       Gerrit](./internal/gerrit/gerritfake).
     - [internal/changelist](./internal/changelist): encapsulates a changelist
       (a.k.a. CL, patch, Gerrit Change).
     - [internal/acls](./internal/acls): enforces ALCs.
     - [internal/tryjob](./internal/tryjob): manages tryjobs (i.e.
       Buildbucket builds) which are used to verify a CL.
     - [internal/prjmanager](./internal/prjmanager) and sub-packages: decides
       when to start new Runs given the state of all CLs in an individual LUCI
       project.
     - [internal/run](./internal/run) and sub-packages: handles individual Run
       from its creation to completion.
     - [internal/rpc](./internal/rpc) pRPC handlers, including "admin" API not
       exposed in public `api` dir.

## Developer Guide

### Error handling guide


Like all other LUCI go code:

  * SHOULD create errors with
    [`errors.Fmt`](https://pkg.go.dev/go.chromium.org/luci/common/errors#Fmt),
    [`errors.New`](https://pkg.go.dev/go.chromium.org/luci/common/errors#New),
    or
    [`errors.WrapIf`](https://pkg.go.dev/go.chromium.org/luci/common/errors#WrapIf)
    to provide additional context. Unlike other error-wrapping packages, it
    also captures the call stack.

  * SHOULD avoid ignoring errors. If it's justified, add a comment in code and
    if necessary log the error.

  * SHOULD use
    [`appstatus`](https://pkg.go.dev/go.chromium.org/luci/grpc/appstatus)
    on errors in RPC-handling codepaths.

LUCI CV also follows these conventions, which in some cases differ from other
LUCI Go code:

  * SHOULD use CV's `common.LogError(ctx, err)` for logging error + stack trace
    instead of `errors.Log(ctx, err)`.
    The CV's version packs the entire stack into as few log entries as possible,
    leading to a better debugging experience with Cloud Logging.

  * Tag the error with `transient.Tag` if the function call can succeed on a
    retry. For example, most Datastore errors different from NoSuchEntity
    SHOULD be thus tagged.

  * SHOULD use [`common.TQifyError`
    ](https://pkg.go.dev/go.chromium.org/luci/cv/internal/common#TQifyError)
    or its custom version `common.TQIfy{...}.Error(ctx, err)` before returning
    from a TQ task handler. This func logs the error with appropriate severity
    and with stack trace as necessary (via `common.LogError`). Additionally,
    it converts the error to an appropriate `server/tq` tag as needed.
    The custom `common.TQIfy` SHOULD be used to reduce noise in
    logs and oncall alerting from the frequent errors during normal operation
    which don't critically harm the service, e.g. "ErrStaleGerritData".

  * MAY panic when a function precondition fails, e.g. sqrt(x) may panic if
    x is negative. This is like an assert in Python / C / C++, which is contrary
    to Go's standard guidance.

  * MUST return a singular error from a function instead of
    `errors.MultiError` or similar multi-error holders unless all of the below
    hold true, which was so far very rare in CV code:

      * the function signature clearly states that a multi-error is returned,
        e.g., `func loadMany(clids ... int64) ([]*CL, errors.MultiError)`;
      * the caller must examine each individual error, e.g. choosing to create
        missing CLs based on loadMany() errors;
      * no transient.Tag or annotation attached to the mutli-error itself, though
        an individual sub-error may have them.

  * SHOULD use [`common.MostSevereError`
    ](https://pkg.go.dev/go.chromium.org/luci/cv/internal/common#MostSevereError)
    on a multi-error before returning. For example, a common pattern to choose 1
    error after parallelizing is
    `return common.MostSevereError(parallel.FanOut(...))`;
      * Individual errors which are thus ignored MAY be logged. This isn't
        required because in practice most such errors are usually correlated,
        and the most severe one is thus typically sufficient for debugging.

### Full end to end testing of new features

 1. Land your code, which gets auto-deployed to `luci-change-verifier-dev` project,
    You may also just upload a tainted version and switch all traffic to it, but
    beware that the next auto-deployment will override it. Thus, adding unit-
    and e2e tests with fake Gerrit and Cloud dependencies is a good first step
    and at times cheaper/faster to do.

 2. `cq-test` LUCI project can be used for any such tests. It's already
    connected to *only* `luci-change-verifier-dev`. The `cq-test` project's
    config tells CV to watch 2 repositories:
      * https://chromium.googlesource.com/playground/gerrit-cq/
      * https://chrome-internal.googlesource.com/playground/cq/
          * This repo also hosts [the LUCI
            config](https://chrome-internal.googlesource.com/playground/cq/+/refs/heads/main/infra/config/main.star)
            for the `cq-test`.

    Creating a CL on `refs/heads/main` will use **combinable** config group,
    meaning multi-CL Runs in ChromeOS style can be created.
    For single-CL Runs, create CLs on `refs/heads/single` ref:

            git new-branch --upstream origin/single
            echo "fail, please" > touch_to_fail_tryjob
            echo "fail with INFRA_FAILURE, please" > touch_to_infra_fail_tryjob
            git commit -a -m "test a signle CL dry run with failing tryjobs"
            git cl upload -d

    You can see recent Runs in
    https://luci-change-verifier-dev.appspot.com/ui/recents/cq-test.

3. (Optional) [internal/cvtesting/e2e/manual](./internal/cvtesting/e2e/manual)
   contains tools to automate common tasks, e.g. creating and CQ-ing large CL
   stacks. Feel free to contribute :)


### UI

tl;dr

```
cd appengine
go run main.go
# See output for URLs to http URLs.
```

To work with Runs from the -dev project, connect to its Datastore by adding
these arguments:

```
go run main.go \
    -http-addr localhost:8800
    -cloud-project luci-change-verifier-dev \
    -root-secret devsecret://base64anything \
    -primary-tink-aead-key devsecret-gen://tink/aead
```

**NOTE**: if you want the old page tokens to work on subsequent `go run main.go ...`
invocations, when you first invoke it, observe output of the first invocation which
should mention a `devsecret://veeeeeeeeeeeeeeery-looooooong-base64-line`, which
you can use on subsequent invocations instead of `devsecret-gen://tink/aead`.

For a quick check, eyeball these two pages:
  * http://localhost:8800/ui/run/cq-test/8991920303854-1-19bc46b6c6972e90
  * http://localhost:8800/ui/recents/cq-test

Finally, you can deploy your work-in-progress to -dev project directly.

## How does this code end up in production?

tl;dr this the usual way for LUCI GAE apps. Roughly,

  1. CL lands in `luci-go` (this) repo.

  1. Autoroller rolls it into `infra/infra` repo
     ([example](https://crrev.com/9ffe1ba58c936d4edf5852da63c470325cf490e8)).

       * *pro-tip*: If autoroller is stuck, you can create your own DEPS roll
         with `roll-dep go/src/go.chromium.org/luci` in your
         `infra/infra` checkout.

  1. Tarball with only the necessary files is created by the Google-internal
     `infra-gae-tarballs-continuous`
     [builder](https://ci.chromium.org/p/infra-internal/builders/prod/infra-gae-tarballs-continuous).

  1. Newest tarball is automatically rolled into also Google-internal `infradata-gae`
     [repo](https://chrome-internal.googlesource.com/infradata/gae/)
     ([example](https://chrome-internal.googlesource.com/infradata/gae/+/e2dc66dbdff92a9dde31758f3f1945758bbaa39c)).

  1. Google-internal `gae-deploy`
     [builder](https://ci.chromium.org/p/infradata-gae/builders/ci/gae-deploy)
     auto-deploys the newest tarball to `luci-change-verifier-dev`
     [GAE project](https://luci-change-verifier-dev.appspot.com/).

  1. Someone manually bumps tarball version to deploy to production
     `luci-change-verifier` via a CL ([example](https://crrev.com/i/4083212)).

       * *Recommended* use `bump-dev-to-prod.py` tool to make such a CL.
         For example,
           ```
           cd gae/app/luci-change-verifier
           ./bump-dev-to-prod.py -- --bug $BUG -r <REVIEWER> -d -s
           ```

  1. The same Google-internal `gae-deploy`
     [builder](https://ci.chromium.org/p/infradata-gae/builders/ci/gae-deploy)
     deploys the desired version to production `luci-change-verifier`
     [GAE project](https://luci-change-verifier.appspot.com/).

## LUCI CV Command Line utils

LUCI Change Verifier provides a command line interface `luci-cv` intended
for LUCI integrators to debug CV configurations.

### Getting the `luci-cv` CLI binary.

There are two ways of getting the latest version of the binary at the moment:

 - Building it yourself from this package: `go.chromium.org/luci/cv/cmd/luci-cv` e.g.

       go build go.chromium/org/luci/cv/cmd/luci-cv

 - Getting it from [CIPD](https://chrome-infra-packages.appspot.com/p/infra/tools/luci-cv)
   e.g. via a command such as:

       cipd ensure -ensure-file - -root cv-cli <<< 'infra/tools/luci-cv/${platform} latest'

For the appropriate syntax and flags information refer to the binary's built-in
documentation. e.g. `luci-cv help match-config`.

### Examples

#### Check if CL is watched by CV and which config group applies

```
# (Assuming luci-cv was installed in the current dir)
~/infra/infra$ ./luci-cv match-config infra/config/generated/commit-queue.cfg https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/3214613

https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/3214613:
  Location: Host: chromium-review.googlesource.com, Repo: infra/luci/luci-go, Ref: refs/heads/main
  Matched: luci-go
```

## Links

 - Additional *Google-internal* docs are found at
   [go/luci/cv](https://goto.google.com/luci/cv).
