default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

node_deps:
	npm install

build: node_deps
	npm run build

release:
	npm ci && ENABLE_GA=true npm run build

test: node_deps
	npm run test

lint: node_deps
	npx gts lint

format: node_deps
	npx gts fix

gen_cert:
	openssl req -x509 -nodes -days 730 -newkey rsa:2048 -keyout dev-configs/cert.key -out dev-configs/cert.pem -config req.cnf -sha256
