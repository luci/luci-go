# LUCI Test Single UI
LUCI Test Single UI is the web UI for all LUCI Test systems.

## Local Development Workflows
### Prerequisites
You need to run the following commands to setup the environment.
```sh
# Activate the infra env (via the infra.git checkout):
cd /path/to/infra/checkout
eval infra/go/env.py

# Install the dependencies.
cd /path/to/this/directory
npm ci
```

### Start a local AppEngine server
TODO: add instructions

### Start a local UI server
To start a [Vite](https://vitejs.dev) local dev server, run
```
npm run dev
```

The local dev server only serves the SPA assets. It sends pRPC and HTTP REST
requests to staging servers (typically hosted on GCP). Check
[.env.development](.env.development) for instructions to configure the target
servers and other local development settings.

#### Login on a local UI server
Currently, there's no easy way to perform a login flow on a local UI server. To
test the page with a logged in session, you can

 - deploy to a personal staging server with the following command, or

   1. `npm run build && yes | gae.py upload -p ../ -A luci-milo-dev default`

 - use an auth state obtained from staging or prod environment with the
   following steps.

   1. open `` `https://${singleUiHost}/ui/search` `` in a browser tab, then
   2. perform a login flow, then
   3. run `copy(JSON.stringify(__STORE.authState.value))` in the browser devtool
   console to copy the auth state, then
   4. paste the output to [auth_state.local.json](auth_state.local.json), then
   5. visit any pages under http://localhost:8080/ui/.

   note that the auth state obtained from a dev environment cannot be used to
      query prod services and vice versa.

### Add a new npm package
You can use [npm install](https://docs.npmjs.com/cli/v8/commands/npm-install) to
add a new npm package.

LUCI Test Single UI uses a private npm registry (defined in [.npmrc](.npmrc)).
By default, it rejects packages that are less than 7 days old (with an
HTTP 451 Unknown error).

You can avoid this issue by temporarily switching to the public npm registry
with the following steps.

 1. comment out the registry setting in [.npmrc](.npmrc), then
 2. install the package, then
 3. replace the registry in all the URLs in the "resolved" field in
 [package-lock.json](package-lock.json) with the private registry, then
 4. uncomment the registry setting in [.npmrc](.npmrc), then
 5. wait 7 days before submitting your CL.

### Others
Check the [Makefile](Makefile), the [parent Makefile](../Makefile), and the
`"scripts"` section in [package.json](package.json) for more available commands.

## Design Decisions
### Service Workers
To improve the loading time, the service workers are added to
 * redirect users from the old URLs to the new URLs (e.g. from
   `` `/b/${build_id}` `` to `` `/ui/b/${build_id}` ``) without hitting the
   server, and
 * cache and serve static assets, including the entry file (index.html), and
 * prefetch resources if the URL matches certain pattern.

It's very hard (if not impossible) to achieve the results above without service
workers because most page visits hit a dynamic (e.g.
`` `/p/${project}/...` ``), and possibly constantly changing (e.g.
`` `/b/${build_id}` ``) URL path, which means cache layers that rely on HTTP
cache headers will not be very effective.
