# Service Workers
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
