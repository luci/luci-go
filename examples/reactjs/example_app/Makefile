default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

# Use the following Make command in the tarball build of your project.
release:
	npm ci && npm run build
