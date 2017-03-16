// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"golang.org/x/net/context"

	"github.com/luci/luci-go/cipd/client/cipd"
	"github.com/luci/luci-go/cipd/client/cipd/ensure"
	"github.com/luci/luci-go/common/isolatedclient"
	"github.com/luci/luci-go/common/logging"
)

// isolateCipdPackages is implementation of cipd2isolate logic.
//
// It takes parsed (but not yet resolved) ensure file, preconfigured
// CIPD and isolated clients, and does all the work.
func isolateCipdPackages(c context.Context, f *ensure.File, cc cipd.Client, ic *isolatedclient.Client) error {
	logging.Infof(c, "TODO")
	return nil
}
