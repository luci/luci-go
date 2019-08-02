## Deployment

Deployment consists of three phases:
* build Docker image locally;
* push Docker image to Google Container Registry;
* apply any deployment configuration changes.

Before starting, ensure the kubectl context is set appropriately with e.g.
```bash
gcloud container cluster get-credentials [cluster name] --project luci-test-results-dev --zone [cluster zone]
kubectl config use-context [correct context]
```

Use `kubectl config view` to determine the correct context.

`make all-prod [LABEL=image-label]` and `make all-dev [LABEL=image-label]`
go through the entire flow of building the local image, labelled as indicated,
uploading it to GCR, and retagging it as the stable image for deployment, but
it's recommended to split the phases up instead.

### Build Docker image

`make build-prod [LABEL=image-label]` and `make build-dev [LABEL=image-label]`
build local Docker images; if no label is provided, `latest` is used.

All images are also tagged by Git hash.

### Upload Docker image

`make upload-prod [LABEL=image-label]` and `make upload-dev [LABEL=image-label]`
upload the indicated local Docker image. Run this when ready to test deployment.

### Deploy Docker image

`make deploy-prod [LABEL=image-label]` and `make deploy-dev [LABEL=image-label]`
apply the `deployment.yaml` configuration, tag the indicated local Docker
image as stable, and patch the deployment with timestamp, rolling out the
updated image. It may take a few minutes for the containers to re-create.

### Misc

`configure` applies the `deployment.yaml` configuration. Note that if there is
no text change to the configuration, no changes will be rolled out (even if
e.g. there is an underlying image change with the same label---hence the
timestamp patch in deployment).
