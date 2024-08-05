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
