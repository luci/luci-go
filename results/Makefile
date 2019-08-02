# Copyright 2019 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

# NB: This Makefile is only intended for dev use!

IMAGE_ID = $(shell git rev-parse HEAD)-tainted

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))
bindir := $(projdir)/bin

gcr_prefix = gcr.io/chops-public-images-dev
image_id = $(gcr_prefix)/$(service):$(IMAGE_ID)

build: .setup-bindir
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v \
		-o $(bindir)/$(service) $(projdir)/k8s/$(service)

image: build
	chmod a+x $(bindir)/$(service)
	docker build \
		-t $(image_id) \
		-f $(projdir)/k8s/$(service)/Dockerfile $(projdir)
	@echo [INFO] built $(image_id)

upload: image
	docker push $(image_id)
	@echo [INFO] pushed $(image_id)

deploy:
	kubectl apply -f $(projdir)/$(service)-deployment.yaml
	# Patch the deployment with a new date to trigger an actual deployment update.
	kubectl patch deployment $(service)-service -p \
		"{\"spec\":{\"template\":{\"metadata\":{\"labels\":{\"date\":\"`date +'%s'`\"}}}}}"

# Helpers
.setup-bindir:
	mkdir -p $(bindir)
