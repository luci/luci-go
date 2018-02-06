LUCI Auth libraries
===================

This package contains auth related elements for LUCI programs.
  * This package - Implements a wrapper around golang.org/x/oauth2 which is
    aware of how LUCI programs handle OAuth2 credentials.
  * `client` - Command-line flags and programs to interact with luci/auth from
    clients of LUCI systems.
  * `identity` - Defines common LUCI identity types and concepts. Used by
    clients and servers.
  * `integration` - Libraries for exporting LUCI authentication tokens (OAuth2)
    to other systems (like gsutil or gcloud).
