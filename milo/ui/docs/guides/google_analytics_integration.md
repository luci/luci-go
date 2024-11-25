# Google Analytics Integration

## Policy
Important note: NO PII SHOULD BE COLLECTED. In particular, no PII should be
used in the page title and URL (including search params).

Before using GA, please take a look at the following links::
 * [go/luci-ui-ga-policy-compliance-checklist](http://go/luci-ui-ga-policy-compliance-checklist)
 * [go/ooga-config](http://go/ooga-config)

## Track page views
To track page view, you can use the `@/generic_libs/components/google_analytics`
package. See the documentation [here](../../src/generic_libs/components/google_analytics/doc.md).

## Track SPA-based redirection
This is useful for collecting data to inform us whether we can turn down the
support for a legacy URL.

To track the SPA-based redirection, you can use `trackedRedirect` from the
`@/generic_libs/tools/router_utils` package. See the documentation on
`trackedRedirect` for details. See [here](https://chromium.googlesource.com/infra/luci/luci-go/+/main/milo/ui/src/core/routes/search_loader/search_redirection_loader.ts#41)
for an example usage.

### Future improvements
 * Before redirecting users to the new route, briefly display a page to instruct
   them to use (e.g. bookmark) the new URL format and notify them that the old
   URL will be turned down soon.

## Useful links
 * GA dashboards
   * [prod](https://analytics.google.com/analytics/web/#/p391270477/reports/intelligenthome)
   * [dev](https://analytics.google.com/analytics/web/#/p391262515/reports/intelligenthome)
 * [go/luci-ui-ga-policy-compliance-checklist](http://go/luci-ui-ga-policy-compliance-checklist)
   * Contains links to privacy scanner, user consent setup, etc.
 * Org policy: [go/ooga-config](http://go/ooga-config)
