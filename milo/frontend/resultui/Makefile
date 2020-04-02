default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

node_deps:
	npm install

build: node_deps
	npm run build

test: build
	npm test

lint:
	npx tslint -p .

format:
	npx tslint -p . --fix
