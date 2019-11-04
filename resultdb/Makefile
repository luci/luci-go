# Copyright 2019 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

# NB: This Makefile is only intended for dev use!

# All calls MUST have service=... argument.
ifeq ($(service),)
  $(error 'service' var is required, e.g. do "make build service=recorder")
endif

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))

# Where Dockerfile is. This entire directory will be sent to the Docker daemon.
dockerdir := $(projdir)/cmd/$(service)/docker
# Put the built binaries under the docker context directory.
bindir := $(dockerdir)/bin

gcr_project = chops-public-images-dev

image_tag = gcr.io/$(gcr_project)/luci/resultdb/$(service):dev

build: .setup-bindir
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v \
		-o $(bindir)/$(service) $(projdir)/cmd/$(service)

test:
	INTEGRATION_TESTS=1 go test -v $(projdir)/cmd/$(service)

image: build
	chmod a+x $(bindir)/$(service)
	docker build -t $(image_tag) $(dockerdir)
	@echo [INFO] built $(image_tag)

upload: image
	docker push $(image_tag)
	@echo [INFO] pushed $(image_tag)

deploy:
	kubectl apply -f $(projdir)/cmd/$(service)/k8s
	# Patch the deployment with a new date to trigger an actual deployment update.
	kubectl patch deployment $(service) -p \
		"{\"spec\":{\"template\":{\"metadata\":{\"labels\":{\"date\":\"`date +'%s'`\"}}}}}"

upload-deploy: upload deploy

# Helpers
.setup-bindir:
	mkdir -p $(bindir)
