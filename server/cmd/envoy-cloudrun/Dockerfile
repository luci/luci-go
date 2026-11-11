# Copyright 2023 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.
FROM envoyproxy/envoy:distroless-v1.39.6

WORKDIR /app

COPY ./envoy.yaml .

# Validate the envoy configuration file (exec form; distroless has no /bin/sh).
RUN ["/usr/local/bin/envoy", "--mode", "validate", "-c", "./envoy.yaml"]

ENTRYPOINT ["/usr/local/bin/envoy", "-c", "./envoy.yaml"]
