# Adding a new host to LUCI UI

LUCI UI provides a minimal number of endpoints itself, for most services the UI
directly calls the other service's host from the browser.

Although the URLs for these hosts could be hardcoded, to simplify using
different configurations in local, dev, and prod environments, these hosts URLs
are made configurable.

If you want to add a new host, you should add it to the existing configuration
and use it from there, rather than hardcoding it somewhere else.

## Files in luci/milo to modify

In each of these files, simply look for an existing host (such as Luci
Bisection) and copy with appropriate modifications.

All paths are relative to the luci/milo directory.

* `proto/config/settings.proto` is the definition of the config format.
* `ui/src/@types/globals.d.ts` defines the types used for typescript
   compilation.
* `httpservice/settings_js_file.go` is an RPC handler that serves the settings
   to the browser.
* `ui/dev_utils/settings_js_utils.ts` imports the settings from environment
   variables into the local dev server.
* `ui/src/testing_tools/setup_after_env.ts` imports the settings from
   environment variables into the test environment.
* `ui/.env.development` defines the default values for the environment
   variables in the local development environment. This file is committed and
   shared by all developers.
* `ui/.env.development.local` overrides the values defined in
   `ui/.env.development`. This file is gitignored. Useful for pointing requests
   to a custom host.

### <mark>Important:</mark>

Due to [push-on-green](http://go/luci-ui-push-on-green), you should land the
code changes above and the config changes in the following section, push them
to prod, **before** you use the newly added host anywhere in the codebase.

The integration tests on the push-on-green builder are run against the
production services. It's **highly recommended** to add at least one simple
integration test for each of your page so that our push-on-green pipeline will
not push the code to production when the required upstream changes have not been
pushed to production.

## Service configuration

You will also need to modify the LUCI UI service configuration in a separate CL
(as the configuration is in a separate repository).

The code changes to luci/milo files must be committed and rolled out to the
environment (dev and/or prod) you are changing the config for **before** you
submit the CL to change the config.

You need to modify one of the following two files to add the configuration.
They are both instances of the `settings.proto` file from the luci/milo
directory.

* Dev - `data/config/configs/luci-milo-dev/settings.cfg`
* Prod - `data/config/configs/luci-milo/settings.cfg`

[Example CL](https://chrome-internal-review.googlesource.com/c/infradata/config/+/6909955/2/configs/luci-milo/settings.cfg)
