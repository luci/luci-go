# Copyright 2023 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

FROM gcr.io/distroless/static:latest

WORKDIR /app

# Copy the built executable binary into Docker.
COPY ./config_server ./

ENTRYPOINT ["./config_server"]
