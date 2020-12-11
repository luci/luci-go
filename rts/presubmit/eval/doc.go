// Copyright 2020 The LUCI Authors.
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

// Package eval implements a framework for selection strategy evaluation.
//
// For evaluation, see
// https://chromium.googlesource.com/infra/luci/luci-go/+/master/rts/doc/eval.md
//
// For an example using this framework, see
// https://source.chromium.org/chromium/infra/infra/+/master:go/src/go.chromium.org/luci/rts/cmd/rts-random/main.go
package eval
