// Copyright 2026 The LUCI Authors.
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

import { getDisplayName } from './alias';

describe('getDisplayName', () => {
  it('maps os for Mac versions', () => {
    expect(getDisplayName('Mac-10.15.7', 'os')).toBe(
      'macOS 10.15 Catalina (Mac-10.15.7)',
    );
    expect(getDisplayName('Mac-14', 'os')).toBe('macOS 14 Sonoma (Mac-14)');
    expect(getDisplayName('Mac-15.1', 'os')).toBe(
      'macOS 15 Sequoia (Mac-15.1)',
    );
    expect(getDisplayName('Mac-26', 'os')).toBe('macOS 26 Tahoe (Mac-26)');
    expect(getDisplayName('Mac-27', 'os')).toBe(
      'macOS 27 Golden Gate (Mac-27)',
    );
  });

  it('maps os for Windows build numbers', () => {
    expect(getDisplayName('Windows-11-26200', 'os')).toBe(
      'Windows 11 version 25H2 (Windows-11-26200)',
    );
  });

  it('maps os for Ubuntu versions', () => {
    expect(getDisplayName('Ubuntu-26.04', 'os')).toBe(
      'Ubuntu 26.04 Resolute Raccoon (Ubuntu-26.04)',
    );
  });

  it('does not affect unaliased os values', () => {
    expect(getDisplayName('Android', 'os')).toBe('Android');
    expect(getDisplayName('Mac-10.9.5', 'os')).toBe('Mac-10.9.5');
  });
});
