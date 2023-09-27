default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

# Note: this is called as part of LUCI Bisection build for
# luci-go-gae-tarballs-continuous. See build/gae/luci-bisection.yaml.
release:
	npm ci && npm run build
