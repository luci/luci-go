# Milo - The interface into luci.

# Layout
* frontend/
  * milo.go - Main router and entry point into app.
  * static/ - CSS and JS assets.  All assets go under a named subfolder.
  * templates/ - HTML assets for use with go templates.
* resp/ - Response structs supported by Milo.
* swarming/ - Files related to scraping pages from swarming.
* testdata/ - Fake data for debug and testing goes here.

# Subdirectory layout
To retain convention, each data source (e.g. swarming, DM) should have the following files:
* endpoints.go - All Google Cloud Endpoints endpoints.
* html.go - All routable HTML endpoints of type handler.
