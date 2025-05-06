export SHELL = /bin/bash

files_no_dist := $(shell git ls-files "[^dist]*.js")

.ONESHELL:

default: _packages-lock.json.done
	npx webpack --mode=development

release: _packages-lock.json.done
	npx webpack --mode=production

serve: _packages-lock.json.done
	npx webpack-dev-server --mode=development

live_demo: _packages-lock.json.done
	npx webpack-dev-server --mode=development --env=demo_type=live

test: _packages-lock.json.done
	npx karma start

browser_test:
	# Good for debugging/developing tests.
	# Starts up the server so one can navigate to http://0.0.0.0:9876 and run the
	# tests there, seeing the results. Unfortunately, due to how karma needs to
	# concatenate the _test.js files and run them through webpack, it doesn't
	# autodetect changes and needs to be re-run manually :(
	npx karma start --single-run=false --auto-watch=true

continuous_test:
	# Good for debugging/developing tests.
	# Runs the tests every time a file changes in ./modules/
	# Works on linux only, with an apt-get install inotify-tools
	make test
	while inotifywait -e modify ./modules/*; do
		make test
	done

format: _packages-lock.json.done
	npx prettier --write $(files_no_dist)
	npx eslint --fix --quiet $(files_no_dist)

_packages-lock.json.done: package-lock.json
	npm ci
	touch _packages-lock.json.done

.PHONY: docs
docs: package-lock.json
	npx jsdoc -c jsdoc.config.js -d docs
	xdg-open docs/index.html

install_deps:
	npm install

# Used by GAE tarball builder to build Swarming Go tarball.
build:
	make install_deps
	make release

# Used by luci-go frontend tester to run Swarming UI tests.
presubmit_test:
	make install_deps
	make test
