# Copyright 2019 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

LABEL ?= latest
IMAGE_ID = $(shell git rev-parse HEAD)

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))
bindir := $(projdir)/bin

service = recorder
gcr_prefix = gcr.io/luci-test-results-dev
image = recorder

deploy: configure
	# Re-tag the indicated image as stable.
	docker pull $(gcr_prefix)/$(image):$(LABEL)
	docker tag $(gcr_prefix)/$(image):$(LABEL) $(gcr_prefix)/$(image):stable
	docker push $(gcr_prefix)/$(image):stable
	# Patch the deployment with a new date to trigger an actual deployment update.
	kubectl patch deployment $(service)-service -p \
		"{\"spec\":{\"template\":{\"metadata\":{\"labels\":{\"date\":\"`date +'%s'`\"}}}}}"

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v \
		-o $(bindir)/$(service) $(projdir)/k8s/$(service)

image: build
	chmod a+x $(bindir)/$(service)
	docker build -t $(gcr_prefix)/$(image):$(IMAGE_ID) \
		-f $(projdir)/k8s/$(service)/Dockerfile $(projdir)
	docker build -t $(gcr_prefix)/$(image):$(LABEL) \
		-f $(projdir)/k8s/$(service)/Dockerfile $(projdir)
	@echo [INFO] built $(gcr_prefix)/$(image):$(LABEL) \($(IMAGE_ID)\)

upload: image
	docker push $(gcr_prefix)/$(image):$(LABEL)
	docker push $(gcr_prefix)/$(image):$(IMAGE_ID)
	@echo [INFO] pushed $(gcr_prefix)/$(image):$(LABEL) \($(IMAGE_ID)\)

configure:
	kubectl apply -f $(projdir)/$(service)-deployment.yaml

all: upload deploy

# Helpers
.setup-bindir:
	rm -rf $(bindir)
	mkdir -p $(bindir)
