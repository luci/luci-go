// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package store

import (
	"testing"

	"github.com/luci/luci-go/common/tsmon/target"
	"golang.org/x/net/context"
)

func TestInMemory(t *testing.T) {
	ctx := context.Background()
	RunStoreImplementationTests(t, ctx, TestOptions{
		Factory: func() Store {
			return NewInMemory(&target.Task{ServiceName: "default target"})
		},
		RegistrationFinished: func(Store) {},
		GetNumRegisteredMetrics: func(s Store) int {
			return len(s.(*inMemoryStore).data)
		},
	})
}
