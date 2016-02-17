// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package store

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/tsmon/target"
)

func TestInMemory(t *testing.T) {
	testStoreImplementation(t, func() Store {
		return NewInMemory(&target.Task{ServiceName: proto.String("default target")})
	})
}
