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

import { cleanup, render, screen } from '@testing-library/react';

import { OutputBuild } from '@/build/types';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuildTable } from '../build_table';
import { BuildTableBody } from '../build_table_body';
import { BuildTableHead } from '../build_table_head';
import { BuildTableRow } from '../build_table_row';

import {
  BuildIdentifierHeadCell,
  BuildIdentifierContentCell,
} from './build_identifier_column';

const buildWithNumber = Build.fromPartial({
  id: '1234',
  number: 12,
  builder: {
    project: 'project',
    bucket: 'bucket',
    builder: 'builder',
  },
}) as OutputBuild;

const buildWithoutNumber = Build.fromPartial({
  id: '2345',
  builder: {
    project: 'project',
    bucket: 'bucket',
    builder: 'builder',
  },
}) as OutputBuild;

describe('GerritChangesContentCell', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    jest.useRealTimers();
  });

  it('can render build with build number', () => {
    render(
      <FakeContextProvider>
        <BuildTable>
          <BuildTableHead>
            <BuildIdentifierHeadCell />
          </BuildTableHead>
          <BuildTableBody>
            <BuildTableRow build={buildWithNumber}>
              <BuildIdentifierContentCell />
            </BuildTableRow>
          </BuildTableBody>
        </BuildTable>
      </FakeContextProvider>,
    );

    const cell = screen.getByRole('cell');
    expect(cell).toHaveTextContent('project/bucket/builder/12');
  });

  it('can render build without build number', () => {
    render(
      <FakeContextProvider>
        <BuildTable>
          <BuildTableHead>
            <BuildIdentifierHeadCell />
          </BuildTableHead>
          <BuildTableBody>
            <BuildTableRow build={buildWithoutNumber}>
              <BuildIdentifierContentCell />
            </BuildTableRow>
          </BuildTableBody>
        </BuildTable>
      </FakeContextProvider>,
    );

    const cell = screen.getByRole('cell');
    expect(cell).toHaveTextContent('project/bucket/builder/b2345');
  });
});
