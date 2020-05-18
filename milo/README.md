# Milo - The UI for LUCI.

Milo is the user interface for LUCI. It displays information from Buildbucket
and ResultDB about builders, builds, and test results, and can be configured to
display custom consoles.

## Releasing

Releases are automatically pushed to luci-milo-dev on commit by the
[gae-deploy](https://ci.chromium.org/p/infradata-gae/builders/ci/gae-deploy)
builder.

To push to prod, the steps are:

1. Get an `infra_internal` checkout
1. `cd data/gae`
1. `vim apps/luci-milo/channels.json`
1. Modify the "stable" version in
   [channels.json](https://chrome-internal.googlesource.com/infradata/gae/+/refs/heads/master/apps/luci-milo/channels.json)
   (e.g. by reusing the current staging version).
1. Run `./main.star` to regenerate the Makefile.
1. Create a CL and add release notes to the description, as follows:
    1. Get the commit hashes from the old and new versions. E.g. in channels.json,
       if you changed the stable version from "31039-7badeba" to "31164-781e143",
       the old commit hash is `7badeba` and the new commit hash is `781e143`. These
       correspond to commits in the
       [infra/infra](https://chromium.googlesource.com/infra/infra/) repo.
    1. Navigate to the DEPS file in the corresponding commits on Gitiles, e.g.
       https://chromium.googlesource.com/infra/infra/+/7badeba/DEPS (old) and
       https://chromium.googlesource.com/infra/infra/+/781e143/DEPS (new).
    1. Check the pinned commit of [infra/luci/luci-go] in the DEPS file at these
       commits. For example, the commits in this case are:
       `2bdb75fedc327ab79d4f514c394a90033f6be375` (old) and
       `a4f26ffd812e4299eacf6d6ae26f01252a88164d` (new).
    1. Run git log between these two commits:
        ```
        git log 2bdb75fedc32..a4f26ffd812e --date=short --first-parent --format='%ad %ae %s'
        ```
    1. Add the resulting command line and output to the CL description. Example:
       https://crrev.com/i/2962041
1. Mail and land the CL.
1. Send an email to luci-releases@ to let people know you've done a new release,
   and link to the push CL.
