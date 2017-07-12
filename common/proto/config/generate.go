// Copyright 2015 The LUCI Authors.
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

// Package config contains luci-config protobuf definitions.
//
// The .proto files are copied from
// https://github.com/luci/luci-py/tree/45f3e27/appengine/components/components/config/proto
// with package name changed to "config" because golint does not like
// underscore in package names.
package config

import (
	"github.com/golang/protobuf/proto"
)

var _ = proto.Marshal

//go:generate cproto
