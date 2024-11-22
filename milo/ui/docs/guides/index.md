# LUCI UI specific development guides

Self link: [go/luci-ui-development-guides](http://go/luci-ui-development-guides).

This doc (and directory) keeps track of a list of **LUCI UI specific development guides**.
General React development guides should not be placed in this directory
(relevant guides may still be referenced here).

## Getting started
 * [VSCode setup recommendation](./vscode_setup_recommendation.md)
 * [Local development workflows](./local_development_workflows.md)

## Development
 * RPC related
   * [Add a new (pRPC) host](./add_new_host.md)
   * [Add a new pRPC service](./add_new_prpc_service.md)
   * [Make (regular/batched/virtualized) pRPC queries](./make_prpc_queries.md)
 * [Effective pagination](./effective_pagination.md)
 * [Make non-overlapping sticky elements](./make_non_overlapping_sticky_elements.md)

## Maintenance
 * [Add/update a NPM dependency](./new_dependencies.md)
 * [Run updated E2E tests in LUCI UI Promoter (i.e. push-on-green pipeline)](./run_updated_e2e_tests_in_luci_ui_promoter.md)
