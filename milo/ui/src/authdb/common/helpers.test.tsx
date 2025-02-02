// Copyright 2024 The LUCI Authors.
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

import { isGlob, isMember } from './helpers';

describe('isGlob validation works', () => {
  test.each([
    ['service:k*nd!', false],
    ['a@example.com', false],
    ['bot:specific_bot_name', false],
    ['*', true],
    ['user:*@example.com', true],
    ['service:*-test-*-pattern', true],
  ])('item is "%s"', (item, expected) => {
    expect(isGlob(item)).toBe(expected);
  });
});

describe('isMember validation works', () => {
  test.each([
    ['unknown:kind', false],
    ['anonymous:user', false],
    ['bot:invalid:character', false],
    ['project:Invalid-uppercase', false],
    ['service:test@example.com', false],
    ['not-an-email', false],
    ['anonymous:anonymous', true],
    ['bot:mega.tron@bots_r-us', true],
    ['project:open-source-123', true],
    ['service:example-service:name', true],
    ['user:test@example.com', true],
    ['test@example.com', true],
  ])('item is "%s"', (item, expected) => {
    expect(isMember(item)).toBe(expected);
  });
});
