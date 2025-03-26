# Add /ui/ path prefix to all UI routes

The `/ui/` prefix was added to address the route conflict between the new
Build/Result UI and the old Build UI when we were re-building the go-template
based build page with Lit. It was intended to be a temporary solution.

Overtime, the `/ui/` prefix is proven to be more useful. It offers a clear
separation between routes handled by the SPA and routes handled by the server.
This separation matters in a few places:

1. Route definition in app.yaml files:
   * With the `/ui/` prefix, we could easily serve all `/ui/` traffic with the
     `index.html` file and let the golang portion handle other paths.
   * Without the `/ui/` prefix, we would need to carefully craft a regex to only
     serve some routes with `index.html`. The regex will either need to be kept
     in sync with the route definition in the SPA, or be kept in sync with the
     route definition on the golang server.
2. Not found page:
   * With the `/ui/` prefix, we could easily serve all unmatched `/ui/*` traffic
     with a SPA based not-found page.
   * Without the `/ui/` prefix, we need to either
      * always hit the server when there's an unmatched route, which is slow and
        causes the user to leave the SPA therefore losing in-memory state/cache.
      * or, the SPA needs to be aware of which routes are handled by the server.
3. Service worker:  
   A service worker's scope is defined by the path prefix they registered at.
   * With the `/ui/` prefix, we could register the main service worker at `/ui/`
     and always serve the user with the cache `index.html` when the user hit
     that route.
   * Without the `/ui/` prefix, we the service worker needs to be aware of which
     routes are handled by the SPA and serve `index.html` accordingly.
       * That said, the server worker is already aware of which routes are
         handled by the SPA. This is required to support cache busting when a
         user visits a newly added page.

It's possible to remove the `/ui/` prefix. But to do so without introducing too
much complexity to our routing definitions, we should reduce the number of
routes handled by the server. This can be achieved by turning down more golang
template based pages, and/or adding prefixes to server handled routes.
