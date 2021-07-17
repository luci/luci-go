# Copyright 2021 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

FROM gcr.io/distroless/static:latest

COPY bin/mailer ./mailer

USER nobody

ENTRYPOINT ["./mailer"]
