# Copyright 2019 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

# NB: This Makefile is only intended for dev use!

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))

# TODO: revisit where binaries are generated and whether to remove this entirely.
bindir := $(projdir)/bin

gcr_project = chops-public-images-dev

image_tag = gcr.io/$(gcr_project)/luci/resultdb/$(service):dev

test-service = INTEGRATION_TESTS=1 go test -v ${projdir}/$(1)

build: .setup-bindir
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v \
		-o $(bindir)/$(service) $(projdir)/cmd/$(service)

image: build
	chmod a+x $(bindir)/$(service)
	docker build \
		-t $(image_tag) \
		-f $(projdir)/cmd/$(service)/Dockerfile \
		$(projdir)
	@echo [INFO] built $(image_tag)

upload: image
	docker push $(image_tag)
	@echo [INFO] pushed $(image_tag)

deploy:
	kubectl apply -f $(projdir)/cmd/$(service)/deployment.yaml
	# Patch the deployment with a new date to trigger an actual deployment update.
	kubectl patch deployment $(service) -p \
		"{\"spec\":{\"template\":{\"metadata\":{\"labels\":{\"date\":\"`date +'%s'`\"}}}}}"

test-recorder:
	$(call test-service,recorder)

# Helpers
.setup-bindir:
	mkdir -p $(bindir)
