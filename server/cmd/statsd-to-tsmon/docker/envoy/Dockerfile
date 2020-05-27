# Copyright 2020 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

FROM gcr.io/distroless/static:latest

COPY bin/statsd-to-tsmon ./statsd-to-tsmon
COPY statsd-to-tsmon.cfg /etc/statsd-to-tsmon/config.cfg

USER nobody

ENTRYPOINT ["./statsd-to-tsmon"]
