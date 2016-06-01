// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package resp

// BuildRef is a MiloBuild with information on how to link to it.
type BuildRef struct {
	URL   string
	Label string
	Build *MiloBuild
}

// MiloBuilder denotes an ordered list of MiloBuilds
type MiloBuilder struct {
	Name           string
	CurrentBuilds  []*BuildRef
	PendingBuilds  []*BuildRef
	FinishedBuilds []*BuildRef

	MachinePool *MachinePool
}

// MachinePool represents the capacity and availability of a builder.
type MachinePool struct {
	Connected int
	Total     int
	Free      int
	Used      int
}
