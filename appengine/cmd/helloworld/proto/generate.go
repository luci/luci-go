// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package helloworld

//go:generate go install github.com/luci/luci-go/tools/cmd/cproto
//go:generate cproto
//go:generate go install github.com/luci/luci-go/tools/cmd/svcdec
//go:generate svcdec -type GreeterServer
