default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

# Called as part of LUCI Analysis build for luci-go-gae-tarballs-continuous.
# See build/gae/luci-analysis.yaml.
release:
	npm ci && npm run build
