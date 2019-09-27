## Deployment

Deployment consists of four phases:
1. build binary locally;
1. build Docker image locally;
1. push Docker image to Google Container Registry;
1. apply any deployment configuration changes.

NOTE: The makefile that provides these four steps is intended only for dev use!

Before starting, ensure the kubectl context is set appropriately with e.g.
```bash
gcloud container cluster get-credentials [cluster name] --project luci-test-results-dev --zone [cluster zone]
kubectl config use-context [correct context]
```

Use `kubectl config view` to determine the correct context.

In each of the below commands, specify the targeted service with
`service=[recorder]` as a parameter.

### `make build`

Build the binary locally for local testing. The resulting binary is in `./bin`.

### `make image`

Build an image with the binary built from local code. The resulting image is
tagged with `[most recent, possibly local commit]-tainted`

Depends on `build`.

### `make upload`

Upload the image built with the binary built from local code. The resulting
image is uploaded to `chops-public-images-dev`.

Depends on `image`.

### `make deploy`

Applies the service deployment .yaml. Make sure to set the image tag to the
correct image!

Because pods will not pick up a different image with the same tag, we patch the
deployment with a timestamp.
