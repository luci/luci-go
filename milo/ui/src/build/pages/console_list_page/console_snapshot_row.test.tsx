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

import { render, screen } from '@testing-library/react';

import { OutputConsoleSnapshot } from '@/build/types';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { ConsoleSnapshot } from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ConsoleSnapshotRow } from './console_snapshot_row';

const consoleSnapshot = ConsoleSnapshot.fromPartial({
  console: {
    id: 'a-console',
    name: 'A Console',
    realm: 'the_project:@root',
    repoUrl: 'https://repo.url',
    builders: [
      {
        id: {
          project: 'the_project',
          bucket: 'the_bucket',
          builder: 'the_builder_1',
        },
        category: 'parent-b|child-1',
      },
      {
        id: {
          project: 'the_project',
          bucket: 'the_bucket',
          builder: 'the_builder_2',
        },
        category: 'parent-a|child',
      },
      {
        id: {
          project: 'the_project',
          bucket: 'the_bucket',
          builder: 'the_builder_3',
        },
        category: 'parent-b|child-2',
      },
    ],
  },
  builderSnapshots: [
    {
      builder: {
        project: 'the_project',
        bucket: 'the_bucket',
        builder: 'the_builder_1',
      },
      build: {
        id: '1000',
        status: Status.SUCCESS,
        builder: {
          project: 'the_project',
          bucket: 'the_bucket',
          builder: 'the_builder_1',
        },
        createTime: '',
      },
    },
    {
      builder: {
        project: 'the_project',
        bucket: 'the_bucket',
        builder: 'the_builder_2',
      },
      // No build for this builder.
      build: undefined,
    },
    {
      builder: {
        project: 'the_project',
        bucket: 'the_bucket',
        builder: 'the_builder_3',
      },
      build: {
        // A build without a build ID but with a build num.
        id: '',
        number: 2,
        status: Status.FAILURE,
        builder: {
          project: 'the_project',
          bucket: 'the_bucket',
          builder: 'the_builder_3',
        },
        createTime: '',
      },
    },
  ],
}) as OutputConsoleSnapshot;

describe('ConsoleSnapshotRow', () => {
  it('should render correctly', async () => {
    render(
      <FakeContextProvider>
        <table>
          <tbody>
            <ConsoleSnapshotRow snapshot={consoleSnapshot} />
          </tbody>
        </table>
      </FakeContextProvider>,
    );

    expect(screen.getByTestId('console-url')).toHaveAttribute(
      'href',
      '/p/the_project/g/a-console/console',
    );
    expect(screen.getByTestId('repo-url')).toHaveAttribute(
      'href',
      'https://repo.url',
    );

    const builderSnapshots = screen.getByTestId('builder-snapshots');
    expect(builderSnapshots.childNodes.length).toEqual(3);

    expect(builderSnapshots.childNodes.item(0)).toHaveClass('success-cell');
    expect(builderSnapshots.childNodes.item(0)).toHaveAttribute(
      'href',
      '/ui/p/the_project/builders/the_bucket/the_builder_1/b1000',
    );

    expect(builderSnapshots.childNodes.item(1)).toHaveClass('failure-cell');
    expect(builderSnapshots.childNodes.item(1)).toHaveAttribute(
      'href',
      '/ui/p/the_project/builders/the_bucket/the_builder_3/2',
    );

    // The second builder should be rendered last because its category is
    // discovered last.
    expect(builderSnapshots.childNodes.item(2)).toHaveClass('no-build-cell');
    expect(builderSnapshots.childNodes.item(2)).toHaveAttribute(
      'href',
      '/ui/p/the_project/builders/the_bucket/the_builder_2',
    );
  });
});
