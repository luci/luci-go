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

import { render, screen } from '@testing-library/react';
import { act } from 'react';
import { VirtuosoMockContext } from 'react-virtuoso';

import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import {
  BatchRequest,
  BatchRequest_Request,
  BatchResponse,
  BatchResponse_Response,
  BuildsClientImpl,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuilderTable } from './builder_table';

const builders = Array(1000)
  .fill(0)
  .map((_, i) =>
    BuilderID.fromPartial({
      project: 'proj',
      bucket: 'bucket',
      builder: `builder${i}`,
    }),
  );

describe('<BuilderTable />', () => {
  let batchMock: jest.SpiedFunction<BuildsClientImpl['Batch']>;

  beforeEach(() => {
    jest.useFakeTimers();
    batchMock = jest.spyOn(BuildsClientImpl.prototype, 'Batch');
  });

  afterEach(() => {
    batchMock.mockReset();
    jest.useRealTimers();
  });

  it('should not render items that are out of bound', async () => {
    render(
      <FakeContextProvider>
        <VirtuosoMockContext.Provider
          value={{ viewportHeight: 400, itemHeight: 40 }}
        >
          <BuilderTable builders={builders} numOfBuilds={10} />
        </VirtuosoMockContext.Provider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());
    expect(screen.queryByText('proj/bucket/builder0')).toBeInTheDocument();
    expect(screen.queryByText('proj/bucket/builder5')).toBeInTheDocument();
    expect(screen.queryByText('proj/bucket/builder10')).not.toBeInTheDocument();
  });

  it('should batch calls together', async () => {
    batchMock.mockResolvedValue(
      BatchResponse.fromPartial({
        responses: Object.freeze(
          Array(10)
            .fill(0)
            .flatMap((_, i) => [
              BatchResponse_Response.fromPartial({
                searchBuilds: {
                  builds: Object.freeze(
                    Array(i)
                      .fill(0)
                      .map(() => Build.fromPartial({})),
                  ),
                },
              }),
              BatchResponse_Response.fromPartial({
                searchBuilds: {
                  builds: Object.freeze(
                    Array(i + 1)
                      .fill(0)
                      .map(() => Build.fromPartial({})),
                  ),
                },
              }),
              BatchResponse_Response.fromPartial({
                searchBuilds: {
                  builds: Object.freeze(
                    Array(10)
                      .fill(0)
                      .map((_, j) =>
                        Build.fromPartial({ id: (10 * i + 5 + j).toString() }),
                      ),
                  ),
                },
              }),
            ]),
        ),
      }),
    );

    render(
      <FakeContextProvider>
        <VirtuosoMockContext.Provider
          value={{ viewportHeight: 400, itemHeight: 40 }}
        >
          <BuilderTable builders={builders} numOfBuilds={10} />
        </VirtuosoMockContext.Provider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(batchMock).toHaveBeenCalledTimes(1);
    expect(batchMock).toHaveBeenCalledWith(
      BatchRequest.fromPartial({
        requests: Object.freeze(
          Array(10)
            .fill(0)
            .flatMap((_, i) => [
              BatchRequest_Request.fromPartial({
                searchBuilds: {
                  predicate: {
                    builder: {
                      project: 'proj',
                      bucket: 'bucket',
                      builder: `builder${i}`,
                    },
                    status: Status.SCHEDULED,
                  },
                  mask: {
                    fields: Object.freeze(['status']),
                  },
                },
              }),
              BatchRequest_Request.fromPartial({
                searchBuilds: {
                  predicate: {
                    builder: {
                      project: 'proj',
                      bucket: 'bucket',
                      builder: `builder${i}`,
                    },
                    status: Status.STARTED,
                  },
                  mask: {
                    fields: Object.freeze(['status']),
                  },
                },
              }),
              BatchRequest_Request.fromPartial({
                searchBuilds: {
                  predicate: {
                    builder: {
                      project: 'proj',
                      bucket: 'bucket',
                      builder: `builder${i}`,
                    },
                    status: Status.ENDED_MASK,
                  },
                  pageSize: 10,
                  mask: {
                    fields: Object.freeze(['status', 'id']),
                  },
                },
              }),
            ]),
        ),
      }),
    );

    const builder5 = screen.getByRole('row', {
      name: /proj\/bucket\/builder5/,
    });
    expect(builder5.querySelector('.pending-cell')).toHaveTextContent(
      '5 pending',
    );
    expect(builder5.querySelector('.running-cell')).toHaveTextContent(
      '6 running',
    );

    const builds = builder5.querySelectorAll('.cell.build');
    expect(builds).toHaveLength(10);
    expect(builds![0]).toHaveAttribute('href', '/ui/b/55');
    expect(builds![5]).toHaveAttribute('href', '/ui/b/60');
  });
});
