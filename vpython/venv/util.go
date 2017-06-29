// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package venv

import (
	"os"

	"github.com/golang/protobuf/proto"

	"github.com/luci/luci-go/common/errors"
)

func writeTextProto(path string, msg proto.Message) error {
	fd, err := os.Create(path)
	if err != nil {
		return errors.Annotate(err, "failed to create output file").Err()
	}

	if err := proto.MarshalText(fd, msg); err != nil {
		_ = fd.Close()
		return errors.Annotate(err, "failed to output text protobuf").Err()
	}

	if err := fd.Close(); err != nil {
		return errors.Annotate(err, "failed to Close temporary file").Err()
	}

	return nil
}
