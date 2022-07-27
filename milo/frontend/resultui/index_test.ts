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

/**
 * @fileoverview
 * This file serves as a single entry to all the test files.
 * It can
 * 1. reduce the time to compile all test bundles, and
 * 2. allow us to use relative source map file path (karma gets confused when
 * trying to map a file that is not at the project root).
 */

import { configure } from 'mobx';

// TODO(crbug/1347294): encloses all state modifying actions in mobx actions
// then delete this.
configure({ enforceActions: 'never' });

// require all modules ending in "_test" from the
// src directory and all subdirectories
const testsContext = require.context('./src', true, /_test$/);

testsContext.keys().forEach(testsContext);
