# LUCI Change Verifier

LUCI Change Verifier (CV) is the LUCI microservice that is replacing LUCI
CQDaemon, while providing the same or better functionality.

CV is already responsible for starting and completing the Runs, which includes
CL Submission.

As of August 2021, CV work group is working on second milestone moving the
remaining functionalities off CQDaemon and onto CV (tracking bug
https://crbug.com/1225047).

TODO(crbug.com/1225047): update this doc.

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

## Developer Guide.

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
go run main.go -cloud-project luci-change-verifier-dev`
```

For a quick check, eyeball these two pages (your port may be different):
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

 - The legacy CQDaemon code is in
   [infra_internal](https://chrome-internal.googlesource.com/infra/infra_internal/+/main/infra_internal/services/cq/README.md).
 - Additional *Google-internal* docs are found at
   [go/luci/cq](https://goto.google.com/luci/cq).
