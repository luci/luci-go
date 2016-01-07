## Usage

Run `backfill <command>`! You can `backfill help buildbot` for more detailed
docs for each command.

### buildbot

The `buildbot` subcommand gets data from CBE and puts it to the datastore. It
gets build information for the most recently executed builds (last ~200 builds).
It also tries to add revision information, with complete generation numbers, if
it can. If it can't find the generation number, though, it still adds revision
information to the build, but with a generation number of -1.

_Note_: NOT for production use, since it depends on CBE. This will hopefully be
deleted after we have tranisitioned off buildbot, and onto buildbucket/DM.

## Enabling remote API
You need to deploy some module to app engine which has the remote api enabled
in order for this backfiller to work successfully.
Full docs here: https://cloud.google.com/appengine/docs/go/tools/remoteapi

You *should* be able to just `goapp deploy` in `/appengine/cmd/milo/remote_api`,
and then do `backfill buildbot -remoteURL
remote-api-1-dot-luci-milo.appspot.com`, or something like that.
