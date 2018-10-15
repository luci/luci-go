// Copyright 2018 The LUCI Authors.
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

package pkg

import (
	"io"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/common"
)

// Instance represents a binary CIPD package file (with manifest inside).
type Instance interface {
	// Pin identifies package name and concreted instance ID of this package file.
	Pin() common.Pin

	// Files returns a list of files to deploy with the package.
	//
	// One of them (named ManifestName) is the manifest file.
	Files() []fs.File

	// DataReader returns reader that reads raw package data.
	DataReader() io.ReadSeeker
}
