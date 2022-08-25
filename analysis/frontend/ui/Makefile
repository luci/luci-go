default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

# Called as part of Weetbix build for infra-gae-tarballs-continuous.
# See infra/build/gae/infra/weetbix.yaml.
release:
	npm ci && npm run build
