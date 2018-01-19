// Copyright 2017 The LUCI Authors.
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

// Package gs implement Google Storage API wrapper used by CIPD backend.
//
// We don't use "cloud.google.com/go/storage" because it doesn't expose stuff
// we need (like resumable uploads and ReaderAt implementation), but instead
// adds a ton of stuff we don't need.
package gs
