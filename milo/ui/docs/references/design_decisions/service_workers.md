# Service Workers

## Benefits

To improve the loading time, the service workers are added to

* redirect users from the old URLs to the new URLs (e.g. from
  `` `/b/${build_id}` `` to `` `/ui/b/${build_id}` ``) without hitting the
  server, and
* cache and serve static assets, including the entry file (index.html), and
* prefetch resources if the URL matches certain pattern.

It's very hard (if not impossible) to achieve the results above without service
workers due to the following reasons:

* Most page visits hit a dynamic (e.g. `` `/p/${project}/...` ``), and possibly
  constantly changing (e.g. `` `/b/${build_id}` ``) URL path, which means cache
  layers that rely on HTTP cache headers will not be very effective.
* There's an unavoidable latency when initializing HTML/JS/CSS bundles
  (100~200ms). Any DOM side JS based prefetch solution will not perform as well
  as the service worker based solution because of that.

## Future plans

### pRPC Prefetch

Currently, the pRPC prefetch is implemented in `@/common/service_workers/prefetch`.
The implementation is very adhoc. To get the benefit, we need to define a logic
that extracts information from the URL and trigger prefetches accordingly.

* This is complicated to setup.
* It's very prone to URL/query changes.

In theory, it should be possible to implement a generic solution that can
deliver this benefit (100~200ms rendering latency improvement) to most pages
(with a pRPC request in its critical rendering path) without requiring adhoc and
complicated setup for each page.

On a high level, one approach is the following:

1. For every page, expose a hook with a predefined name that
   * Read needed params from the URL.
   * Trigger the queries and return the results.
2. In the page implementation, use that hook to send queries.
3. When called in a different environment, instead of sending out the queries,
   the hook can just report back a mapping from URL to queries that need to be
   sent.
4. Make a function that traverses the routes definition, visit all hook
   definition, and compute a mapping between URL to queries that need to be
   sent.
5. In the service worker, use the mapping we obtained from the previous step to
   perform prefetches.
