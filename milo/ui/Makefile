default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

# Note: prefer using a Makefile command over package.json scripts when running
# commands in bots so we can document the usage of those commands. This prevents
# us from accidentally breaking commands that a bot relies on.

lint:
	npm run lint

format:
	npm run lint -- --fix

# Used to run unit tests in a builder.
# The acutal builder uses `npm run test` directly. Still document it here so we
# less likely to accidentally change `test` script and break the unit test
# builder.
test:
	npm run test

# Used to build
# * release tarballs (see https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/build/gae/luci-milo-ui.yaml).
# * preview server assets when running integration tests.
build:
	npm run build

e2e:
	npm run e2e

# Provided for convience.
# See up-dev in `../Makefile` for details.
up-dev:
	cd ../ && make up-dev

# Deploys a UI demo.
#
# Comapring to `up-dev`, this is much faster because it only deploys the default
# service and does not run `npm ci` (which means you need to ensure the
# dependencies are installed).
# It also overrides the Milo host when deploying the default service only for UI
# demo so the UI does not talk to the demo version of the Milo API service which
# may not exist.
deploy-ui-demo:
	export VITE_OVERRIDE_MILO_HOST=staging.milo.api.luci.app && npm run build
	gae.py upload -p ../ -A luci-milo-dev ui -f
