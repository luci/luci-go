// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package lucictx

import (
	"fmt"

	"golang.org/x/net/context"
)

// Swarming is a struct that may be used with the "swarming" section of
// LUCI_CONTEXT.
type Swarming struct {
	SecretBytes []byte `json:"secret_bytes"`
}

// GetSwarming calls Lookup and returns the current Swarming from LUCI_CONTEXT
// if it was present. If no Swarming is in the context, this returns nil.
func GetSwarming(ctx context.Context) *Swarming {
	ret := Swarming{}
	ok, err := Lookup(ctx, "swarming", &ret)
	if err != nil {
		panic(err)
	}
	if !ok {
		return nil
	}
	return &ret
}

// SetSwarming Sets the Swarming in the LUCI_CONTEXT.
func SetSwarming(ctx context.Context, swarm *Swarming) context.Context {
	ctx, err := Set(ctx, "swarming", swarm)
	if err != nil {
		panic(fmt.Errorf("impossible: %s", err))
	}
	return ctx
}
