# CIPD backend

Code layout:
  * `frontend`, `backend`, `static` - entry points for GAE modules.
  * `impl` - the root package with implementation guts of all APIs.
  * `ui` - web UI implementation.

Deployment:

```shell
gae.py upload -A chrome-infra-packages-dev
gae.py switch -A chrome-infra-packages-dev
```

This implementation replaced Python one, that used to live in infra.git
repository. It has been removed in
[this commit](https://chromium.googlesource.com/infra/infra/+/a7759c5f0).
