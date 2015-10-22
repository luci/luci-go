luci-go: `apigen` Tool Examples
==========================================

This package contains services that demonstrate the usage and output of the
`apigen` tool.

Code layout
-----------

  * [/appengine/apigen_examples/dumb_counter/...](
  https://github.com/luci/luci-go/tree/master/appengine/apigen_examples/dumb_counter)
    contains a `dumb_counter` AppEngine v1 service, which implements a basic
    counter REST API using Cloud Endpoints.
  * [/appengine/apigen_examples/dumb_counter_mvm/...](
  https://github.com/luci/luci-go/tree/master/appengine/apigen_examples/dumb_counter_mvm)
    implements the `dumb_counter` service as an AppEngine v2 Managed VM.
