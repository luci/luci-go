# Copyright 2023 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

FROM gcr.io/distroless/static:latest

COPY bin/selfsigned-cert ./selfsigned-cert

USER nobody

ENTRYPOINT ["./selfsigned-cert"]
