GAE_PY = ../../../../../luci/appengine/components/tools/gae.py

default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

format:
	gofmt -s -w .

test:
	go test ./...

ui-test:
	cd frontend/ui; \
	npm run jest

build-frontend:
	cd frontend/ui; \
	make release

deploy: build-frontend
	$(GAE_PY) upload --target-version ${USER} -A luci-bisection-dev
