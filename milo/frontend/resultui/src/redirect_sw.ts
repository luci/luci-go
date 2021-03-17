var sw = self as unknown as ServiceWorkerGlobalScope;

sw.addEventListener('fetch', function(event) {
  const url = new URL(event.request.url);
  const isNewBuildPage = url.pathname.match(/^\/b\/[^/]+(\/.*)?/)
    ||url.pathname.match(/^\/p\/[^/]+\/builders\/[^/]+\/[^/]+\/[^/]+(\/.*)?/);
  if (isNewBuildPage) {
    url.pathname = '/ui' + url.pathname;
    event.respondWith(Response.redirect(url.toString()));
    return;
  }
  event.respondWith(fetch(event.request));
});
