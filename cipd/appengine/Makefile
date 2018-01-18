APP_DIR := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

default: help

help:
	@echo "Available commands:"
	@sed -n '/^[a-zA-Z0-9_]*:/s/:.*//p' <Makefile

devserver:
	dev_appserver.py \
		${APP_DIR}/dispatch.yaml \
		${APP_DIR}/frontend/app.yaml \
		${APP_DIR}/frontend/module-frontend.yaml \
		${APP_DIR}/backend/module-backend.yaml \
		${APP_DIR}/static/module-static.yaml \
		-A chrome-infra-packages-dev --skip_sdk_update_check

deploy_staging:
	gcloud app deploy \
		${APP_DIR}/dispatch.yaml \
		${APP_DIR}/frontend/index.yaml \
		${APP_DIR}/frontend/cron.yaml \
		${APP_DIR}/frontend/queue.yaml \
		${APP_DIR}/frontend/module-frontend.yaml \
		${APP_DIR}/backend/module-backend.yaml \
		${APP_DIR}/static/module-static.yaml \
		--project chrome-infra-packages-dev \
		--version dev \
		--no-promote \
		--no-stop-previous-version
