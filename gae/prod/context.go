// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"golang.org/x/net/context"
)

// Use adds implementations for the following gae interfaces to the
// context:
//   * gae.RawDatastore
//   * gae.TaskQueue
//   * gae.Memcache
//   * gae.GlobalInfo
//
// These can be retrieved with the gae.Get functions.
//
// The implementations are all backed by the real appengine SDK functionality,
func Use(c context.Context) context.Context {
	return useRDS(useMC(useTQ(useGI(c))))
}
