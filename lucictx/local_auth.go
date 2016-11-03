// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package lucictx

import (
	"fmt"

	"golang.org/x/net/context"
)

// LocalAuth is a struct that may be used with the "local_auth" section of
// LUCI_CONTEXT.
type LocalAuth struct {
	RPCPort uint32 `json:"rpc_port"`
	Secret  []byte `json:"secret"`
}

// GetLocalAuth calls Lookup and returns the current LocalAuth from LUCI_CONTEXT
// if it was present. If no LocalAuth is in the context, this returns nil.
func GetLocalAuth(ctx context.Context) *LocalAuth {
	ret := LocalAuth{}
	ok, err := Lookup(ctx, "local_auth", &ret)
	if err != nil {
		panic(err)
	}
	if !ok {
		return nil
	}
	return &ret
}

// SetLocalAuth Sets the LocalAuth in the LUCI_CONTEXT.
func SetLocalAuth(ctx context.Context, la *LocalAuth) context.Context {
	ctx, err := Set(ctx, "local_auth", la)
	if err != nil {
		panic(fmt.Errorf("impossible: %s", err))
	}
	return ctx
}
