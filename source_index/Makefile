GAE_PY = ../../../../../luci/appengine/components/tools/gae.py

default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

format:
	gofmt -s -w .

install-spanner-emulator:
	gcloud components install cloud-spanner-emulator -q

test:
	INTEGRATION_TESTS=1 go test -race ./...

deploy:
	$(GAE_PY) upload -A luci-source-index-dev -f
