// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package deps

import (
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/grpc/grpcutil"
	"google.golang.org/grpc/codes"
)

func grpcAnnotate(err error, code codes.Code, reason string, args ...interface{}) *errors.Annotator {
	return errors.Annotate(err, reason, args...).Tag(grpcutil.Tag.With(code))
}
