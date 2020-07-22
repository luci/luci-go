#!/usr/bin/env bash

Egcloud beta emulators spanner start & gcloud spanner instances create testing --config=emulator-config --nodes=1

