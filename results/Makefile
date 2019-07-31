# Copyright 2019 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

LABEL ?= latest
IMAGE_ID = $(shell git rev-parse HEAD)

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))

deployment_prefix = skeleton-service
gcr_prefix = gcr.io/luci-test-results
image = helloworld

deploy-prod: configure
	# Re-tag the indicated image as stable (on prod).
	docker pull $(gcr_prefix)/$(image):$(LABEL)
	docker tag $(gcr_prefix)/$(image):$(LABEL) $(gcr_prefix)/$(image):stable
	docker push $(gcr_prefix)/$(image):stable
	# Patch the deployment with a new date to trigger an actual deployment update.
	kubectl patch deployment $(deployment_prefix) -p \
		"{\"spec\":{\"template\":{\"metadata\":{\"labels\":{\"date\":\"`date +'%s'`\"}}}}}"

deploy-dev: configure
	# Re-tag the indicated image as stable (on dev).
	docker pull $(gcr_prefix)-dev/$(image):$(LABEL)
	docker tag $(gcr_prefix)-dev/$(image):$(LABEL) $(gcr_prefix)-dev/$(image):stable
	docker push $(gcr_prefix)-dev/$(image):stable
	# Patch the deployment with a new date to trigger an actual deployment update.
	kubectl patch deployment $(deployment_prefix)-dev -p \
		"{\"spec\":{\"template\":{\"metadata\":{\"labels\":{\"date\":\"`date +'%s'`\"}}}}}"

build-prod:
	docker build -t $(gcr_prefix)/$(image):$(IMAGE_ID) -f $(projdir)/k8s/Dockerfile.prod $(projdir)/k8s
	docker build -t $(gcr_prefix)/$(image):$(LABEL) -f $(projdir)/k8s/Dockerfile.prod $(projdir)/k8s
	@echo [INFO] built $(gcr_prefix)/$(image):$(LABEL) \($(IMAGE_ID)\)

build-dev:
	docker build -t $(gcr_prefix)-dev/$(image):$(IMAGE_ID) $(projdir)/k8s
	docker build -t $(gcr_prefix)-dev/$(image):$(LABEL) $(projdir)/k8s
	@echo [INFO] built $(gcr_prefix)-dev/$(image):$(LABEL) \($(IMAGE_ID)\)

upload-prod: build-prod
	docker push $(gcr_prefix)/$(image):$(LABEL)
	docker push $(gcr_prefix)/$(image):$(IMAGE_ID)
	@echo [INFO] pushed $(gcr_prefix)/$(image):$(LABEL) \($(IMAGE_ID)\)

upload-dev: build-dev
	docker push $(gcr_prefix)-dev/$(image):$(LABEL)
	docker push $(gcr_prefix)-dev/$(image):$(IMAGE_ID)
	@echo [INFO] pushed $(gcr_prefix)-dev/$(image):$(LABEL) \($(IMAGE_ID)\)

configure:
	kubectl apply -f $(projdir)/deployment.yaml

all-prod: upload-prod deploy-prod
all-dev: upload-dev deploy-dev
