default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

# Note: prefer using a Makefile command over package.json scripts when running
# commands in bots so we can document the usage of those commands. This prevents
# us from accidentally breaking commands that a bot relies on.

# Used to run lint in a builder (see https://chromium.googlesource.com/infra/infra/+/73ad16cffae6eb25bb30cba6da1a91dc777b2a43/recipes/recipes/infra_frontend_tester.py#127).
lint:
	npm run lint

format:
	npm run lint -- --fix

# Used to run unit tests in a builder (see https://chromium.googlesource.com/infra/infra/+/73ad16cffae6eb25bb30cba6da1a91dc777b2a43/recipes/recipes/infra_frontend_tester.py#135).
# The acutal builder uses `npm run test` directly. Still document it here so we
# less likely to accidentally change `test` script and break the unit test
# builder.
test:
	npm run test

# Used to build
# * release tarballs (see https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/build/gae/luci-milo-ui.yaml).
# * preview server assets when running integration tests.
# * release tarballs when running presubmit checks to ensure the CL doesn't
#   break the builds (see https://chromium.googlesource.com/infra/infra/+/73ad16cffae6eb25bb30cba6da1a91dc777b2a43/recipes/recipes/infra_frontend_tester.py#128).
build:
	NODE_OPTIONS=--max-old-space-size=4096 npm run build

# Regenerate TypeScript protos on the frontend
gen:
	./scripts/gen_proto.sh

# Used to run integration test in a builder (see https://chromium.googlesource.com/infra/infra/+/73ad16cffae6eb25bb30cba6da1a91dc777b2a43/recipes/recipes/luci_ui_promoter.py#85).
e2e:
	npm run e2e

# Provided for convience.
# See up-dev in `../Makefile` for details.
up-dev:
	cd ../ && make up-dev

# Deploys a UI demo.
#
# Comapring to `up-dev`, this is much faster because it only deploys the
# required UI service and does not run `npm ci` (which means you need to ensure
# the dependencies are installed).
# It also overrides the Milo host so the UI does not talk to the demo version of
# the Milo API service which may not exist.
deploy-ui-demo:
	export VITE_OVERRIDE_MILO_HOST=staging.milo.api.luci.app && npm run build
	gae.py upload -p ../ -A luci-milo-dev ui-new -f
