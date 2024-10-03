default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

node_deps:
	npm ci

build: node_deps
	npm run build

# Used when building release tarballs. See
# https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/build/gae/milo.yaml#8
release:
	npm ci && npm run build

test: node_deps
	npm run test

lint: node_deps
	npm run lint

format: node_deps
	npm run lint -- --fix

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
