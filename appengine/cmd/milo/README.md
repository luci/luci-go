# Milo - The interface into luci.

# Layout
* frontend/
  * milo.go - Main router and entry point into app.
  * static/ - CSS and JS assets.  All assets go under a named subfolder.
  * templates/ - HTML assets for use with go templates.
* resp/ - Response structs supported by Milo.
* swarming/ - Files related to scraping pages from swarming.

# Subdirectory layout
To retain convention, each data source (e.g. swarming, DM) should have the following files:
* html.go - All routable HTML endpoints of type handler.

# Themes and Templates
Milo supports the switching of themes (template bundles) based on user
preference.  Themes must follow these layouts:
* Go Templates go under /frontend/templates/[[Theme Name]]
** Base templates go under /frontend/templates/[[Theme Name]]/includes
** Actual templates go under /frontend/templates/[[Theme Name]]/pages
* Static resources (css, javascript, images, etc) go under
  /frontend/static/[[Theme Name]].  This boundry isn't enforced, this is just by
  convention, so one theme is allowed to use resources in other themes (but it
  is not recommended)
* Add the Theme Name into the map in /settings/theme.go:THEMES

# Seeding data for local development
* After starting the dev_appserver, run go run cmd/backfill/main.go buildbot
  -master="chromium.win" -remote-url="localhost:8080" -dryrun=false
  -buildbot-fallback=true (replace the port number with your dev_appserver port
  number)
