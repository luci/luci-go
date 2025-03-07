// Copyright 2025 The LUCI Authors.
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

// Package gitsource implements efficiently cached access to remote git
// repositories.
//
// Currently this is implemented by shelling out to `git` as the canonical
// interface for interacting with the git protocol and data. It's possible that
// at some point later this could/should be implemented directly using something
// like github.com/go-git/go-git.
//
// To use this source, first construct a new Cache at some filesystem path, and
// then use this to obtain Fetchers for different pins. The cache will be
// a `bare` git repo, and all operations will maintain this as a permanent cache
// state.
package gitsource
