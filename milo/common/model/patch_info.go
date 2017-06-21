// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package model

// PatchInfo provides information about a patch included in a build.
type PatchInfo struct {
	// A link to the patch page (i.e. the gerrit/rietveld review page).
	Link Link

	// The email of the author of the Patch.
	AuthorEmail string
}
