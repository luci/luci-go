# Copyright 2019 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

FROM gcr.io/distroless/static:latest

# The binary should be built outside the container and placed in the Docker
# context directory. See Makefile for how it can be done during development.
COPY helloworld .

USER nobody
ENTRYPOINT ["./helloworld"]
