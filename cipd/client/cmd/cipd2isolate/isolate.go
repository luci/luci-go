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
