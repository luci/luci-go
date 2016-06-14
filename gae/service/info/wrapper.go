// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package info

import (
	"strings"

	"golang.org/x/net/context"
)

type infoImpl struct {
	RawInterface
}

var _ Interface = infoImpl{}

func (i infoImpl) MustNamespace(namespace string) context.Context {
	ret, err := i.Namespace(namespace)
	if err != nil {
		panic(err)
	}
	return ret
}

func (i infoImpl) TrimmedAppID() string {
	toks := strings.Split(i.AppID(), ":")
	return toks[len(toks)-1]
}
