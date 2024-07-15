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

import { RestLogResponse } from './types';

const restLogResponse: RestLogResponse = {
  log: [
    {
      commit: 'a154ba2a87d830b7931e47e87400071837cbae75',
      tree: '3a2a91dbcddffbf430cec7cca5111f0ea91cf842',
      parents: ['210ab1304b5bb0c479f3c6d22ccf7a8e83a39a68'],
      author: {
        name: 'Author 0',
        email: 'author@chromium.org',
        time: 'Thu Apr 04 17:40:47 2024',
      },
      committer: {
        name: 'CQ',
        email: 'cq@service-account.com',
        time: 'Thu Apr 04 17:40:47 2024',
      },
      message: 'message 1',
      tree_diff: [
        {
          type: 'modify',
          old_id: '158335b514df0509d2c635598f28063f846d9c04',
          old_mode: 33188,
          old_path: 'path/to/existing/file',
          new_id: '58fe98ff73c3c1211e46d7e900e75d9a7d329df5',
          new_mode: 33188,
          new_path: 'path/to/existing/file',
        },
        {
          type: 'add',
          old_id: '0000000000000000000000000000000000000000',
          old_mode: 0,
          old_path: '/dev/null',
          new_id: '8fb4d57b3f65ffc5df3cce8ab3fbb1e5e5907de6',
          new_mode: 33188,
          new_path: 'path/to/newly/added/file',
        },
      ],
    },
    {
      commit: '210ab1304b5bb0c479f3c6d22ccf7a8e83a39a68',
      tree: 'bd4f9b1a20b20aaaa6e9921f8c9eea8331d68d1e',
      parents: ['1253d8920b6e84d6f0fac26da9cb2d38f4858bd2'],
      author: {
        name: 'Author 1',
        email: 'author1@chromium.org',
        time: 'Thu Apr 04 17:40:03 2024',
      },
      committer: {
        name: 'CQ',
        email: 'cq@service-account.com',
        time: 'Thu Apr 04 17:40:03 2024',
      },
      message: 'msg2',
      tree_diff: [
        {
          type: 'delete',
          old_id: '4bf00673cb69e54adb2f2f30f33df61afa27445f',
          old_mode: 33188,
          old_path: 'path/to/old/file/README.md',
          new_id: '0000000000000000000000000000000000000000',
          new_mode: 0,
          new_path: '/dev/null',
        },
        {
          type: 'rename',
          old_id: '16bc736bb72ebf77ad8a86d531b72f53370231b4',
          old_mode: 33188,
          old_path: 'path/to/old/file',
          new_id: 'f1e6524d34678b80a974b07285a4f92652ad4ea2',
          new_mode: 33188,
          new_path: 'path/to/new/file',
        },
      ],
    },
  ],
  next: '5140ece7b919de65940864f54910477bb3f3162d',
};

const protoLogResponse = {
  log: [
    {
      id: 'a154ba2a87d830b7931e47e87400071837cbae75',
      tree: '3a2a91dbcddffbf430cec7cca5111f0ea91cf842',
      parents: ['210ab1304b5bb0c479f3c6d22ccf7a8e83a39a68'],
      author: {
        name: 'Author 0',
        email: 'author@chromium.org',
        time: '2024-04-04T17:40:47.000Z',
      },
      committer: {
        name: 'CQ',
        email: 'cq@service-account.com',
        time: '2024-04-04T17:40:47.000Z',
      },
      message: 'message 1',
      treeDiff: [
        {
          type: 3,
          oldId: '158335b514df0509d2c635598f28063f846d9c04',
          oldMode: 33188,
          oldPath: 'path/to/existing/file',
          newId: '58fe98ff73c3c1211e46d7e900e75d9a7d329df5',
          newMode: 33188,
          newPath: 'path/to/existing/file',
        },
        {
          type: 0,
          oldId: '0000000000000000000000000000000000000000',
          oldMode: 0,
          oldPath: '/dev/null',
          newId: '8fb4d57b3f65ffc5df3cce8ab3fbb1e5e5907de6',
          newMode: 33188,
          newPath: 'path/to/newly/added/file',
        },
      ],
    },
    {
      id: '210ab1304b5bb0c479f3c6d22ccf7a8e83a39a68',
      tree: 'bd4f9b1a20b20aaaa6e9921f8c9eea8331d68d1e',
      parents: ['1253d8920b6e84d6f0fac26da9cb2d38f4858bd2'],
      author: {
        name: 'Author 1',
        email: 'author1@chromium.org',
        time: '2024-04-04T17:40:03.000Z',
      },
      committer: {
        name: 'CQ',
        email: 'cq@service-account.com',
        time: '2024-04-04T17:40:03.000Z',
      },
      message: 'msg2',
      treeDiff: [
        {
          type: 2,
          oldId: '4bf00673cb69e54adb2f2f30f33df61afa27445f',
          oldMode: 33188,
          oldPath: 'path/to/old/file/README.md',
          newId: '0000000000000000000000000000000000000000',
          newMode: 0,
          newPath: '/dev/null',
        },
        {
          type: 4,
          oldId: '16bc736bb72ebf77ad8a86d531b72f53370231b4',
          oldMode: 33188,
          oldPath: 'path/to/old/file',
          newId: 'f1e6524d34678b80a974b07285a4f92652ad4ea2',
          newMode: 33188,
          newPath: 'path/to/new/file',
        },
      ],
    },
  ],
  nextPageToken: '5140ece7b919de65940864f54910477bb3f3162d',
};

describe('RestLogResponse', () => {
  describe('toProto', () => {
    it('toProto works', async () => {
      expect(RestLogResponse.toProto(restLogResponse)).toEqual(
        protoLogResponse,
      );
    });
  });
});
