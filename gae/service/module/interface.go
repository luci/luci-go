// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package module

// Interface is the interface for all of the package methods which normally
// would be in the 'module' package.
type Interface interface {
	List() ([]string, error)
	NumInstances(module, version string) (int, error)
	SetNumInstances(module, version string, instances int) error
	Versions(module string) ([]string, error)
	DefaultVersion(module string) (string, error)
	Start(module, version string) error
	Stop(module, version string) error
}
