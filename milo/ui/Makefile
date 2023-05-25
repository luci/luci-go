default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

node_deps:
	npm ci

build: node_deps
	npm run build

release:
	npm ci && npm run build

test: node_deps
	npm run test

lint: node_deps
	npx gts lint

format: node_deps
	npx gts fix
