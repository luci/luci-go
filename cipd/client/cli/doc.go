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

// Package cli implements command line interface for CIPD client.
//
// Its main exported function is GetApplication(...) that takes a bundle with
// default parameters and returns a *cli.Application configured with this
// defaults.
//
// There's also Main(...) that does some additional arguments manipulation. It
// can be used to build a copy of 'cipd' tool with some defaults tweaked.
package cli
