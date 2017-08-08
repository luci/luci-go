// Copyright 2017 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package venv

import (
	"os"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"
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
