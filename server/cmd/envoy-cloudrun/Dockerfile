# Copyright 2023 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.
FROM envoyproxy/envoy:v1.34.2

WORKDIR /app

COPY ./envoy.yaml .

# Validate the enovy configuration file.
RUN envoy --mode validate -c ./envoy.yaml

ENTRYPOINT ["envoy", "-c", "./envoy.yaml"]
