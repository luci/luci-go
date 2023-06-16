// Copyright 2023 The LUCI Authors.
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

// Package exec is intended to be a drop-in replacement for "os/exec"
// which can be mocked.
//
// See the docs in `go.chromium.org/luci/common/exec/execmock` for information
// on how mocking exec works.
//
// NOTE: `Command` now takes a context; this context will only be used for
// the purpose of mocking, but still allows you to make a Command which is not
// bound to the lifetime of the context (i.e. will not be automatically killed
// when the context is Done()).
package exec
