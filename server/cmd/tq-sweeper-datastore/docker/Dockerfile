# Copyright 2026 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

FROM gcr.io/distroless/static:latest

COPY bin/tq-sweeper-datastore ./tq-sweeper-datastore

USER nobody

ENTRYPOINT ["./tq-sweeper-datastore"]
