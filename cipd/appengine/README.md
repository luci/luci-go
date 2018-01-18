# CIPD backend

Currently assumed to be deployed on top of Python CIPD backend (to inherit
data in the datastore). Thus modules are named `<something>-go` (to avoid
colliding with similarly named python modules), and requests to them are routed
via dispatch.yaml.

Additionally, cron.yaml, queue.yaml and index.yaml contain definitions for
resourced used by Python code, since we don't want to override them.

Code layout:
  * `frontend`, `backend`, `static` - entry points for GAE modules.
  * `impl` - the root package with all implementation guts.

Python code being replaced:
https://chromium.googlesource.com/infra/infra/+/master/appengine/chrome_infra_packages/
