default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

# Only run npm install if the node_modules folder is missing.
# This is useful for faster builds and tests, but may fail if the node modules
# are present and require updating.
node_deps_if_not_present:
	[ -d node_modules ] || npm install

node_deps:
	npm install

build: node_deps
	npm run build

build_fast: node_deps_if_not_present
	npm run build

test: node_deps
	npm run test

test_fastt: node_deps_if_not_present
	npm run test

lint: node_deps_if_not_present
	npx tslint -p .

format: node_deps_if_not_present
	npx tslint -p . --fix
