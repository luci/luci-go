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

import Hotkeys from 'hotkeys-js';

import './routes';

// Prevent hotkeys from reacting to events from input related elements enclosed in shadow DOM.
Hotkeys.filter = (e) => {
  const tagName = (e.composedPath()[0] as Partial<HTMLElement>).tagName || '';
  return !['INPUT', 'SELECT', 'TEXTAREA'].includes(tagName);
};
