# Copyright 2026 The Chromium Authors
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

NODE_BIN := $(shell command -v node 2>/dev/null || ls $(abspath ../../../../../../../cipd/bin/node) 2>/dev/null || echo node)

.PHONY: gen build-prototype

gen:
	./gen_ts_proto.sh

build-prototype:
	PATH="$(dir $(NODE_BIN)):$$PATH" "$(NODE_BIN)" --experimental-strip-types ./scripts/build_prototype.ts \
		--slug "$${SLUG:-tot-fleet-console}" \
		--title "$${TITLE:-Fleet Console Prototype}" \
		--description "$${DESC:-Standalone Fleet Console static UI mock}" \
		--route "$${ROUTE:-/ui/fleet/p/chromeos/devices}"
