# The Token Server

The token server is responsible for minting short-lived (<1 hour) stateless
access tokens for Swarming bots. It uses PKI to authenticate bots.

Code layout:
  * `frontend`, `backend` - entry points for GAE service binaries.
  * `devcfg` - luci-config config files when running locally.
  * `impl` - the root package with all implementation guts.
