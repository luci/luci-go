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

import { render, screen, waitFor } from '@testing-library/react';
import { useContext } from 'react';
import { useParams } from 'react-router';

import { ReadWorkPlanResponse } from '@/proto/turboci/graph/orchestrator/v1/read_workplan_response.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChronicleContext, DEMO_WORKPLAN_ID } from './context';
import { ChronicleContextProvider } from './provider';

jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: jest.fn(),
}));

const mockUseParams = jest.mocked(useParams);

describe('ChronicleContextProvider', () => {
  let originalFetch: typeof global.fetch;

  beforeEach(() => {
    originalFetch = global.fetch;
    global.fetch = jest.fn();
    mockUseParams.mockReturnValue({ workplanId: DEMO_WORKPLAN_ID });
  });

  afterEach(() => {
    global.fetch = originalFetch;
    jest.resetAllMocks();
  });

  function TestWrapper({ children }: { children: React.ReactNode }) {
    const { workplanId, detecting, detectionFailed } =
      useContext(ChronicleContext);

    if (detecting) {
      return (
        <div>
          Detecting the Turbo CI instance that contains workplan {workplanId}.
        </div>
      );
    }

    if (detectionFailed) {
      return (
        <div>
          Workplan {workplanId} could not be found in any of the Turbo CI
          environments.
        </div>
      );
    }

    return <>{children}</>;
  }

  function TestComponent() {
    const { workplanId, activeEnvironment, graph, failedEnvironments } =
      useContext(ChronicleContext);
    return (
      <div>
        <div data-testid="workplan-id">{workplanId}</div>
        <div data-testid="active-env">{activeEnvironment}</div>
        <div data-testid="graph-exists">{graph ? 'yes' : 'no'}</div>
        <div data-testid="failed-envs">
          {failedEnvironments
            .map((f) => `${f.env.environment}:${f.errorType}`)
            .join(',')}
        </div>
      </div>
    );
  }

  it('renders demo workplan immediately using fake data', async () => {
    render(
      <FakeContextProvider>
        <ChronicleContextProvider>
          <TestWrapper>
            <TestComponent />
          </TestWrapper>
        </ChronicleContextProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByTestId('workplan-id')).toHaveTextContent(
      DEMO_WORKPLAN_ID,
    );
    expect(screen.getByTestId('active-env')).toHaveTextContent(
      DEMO_WORKPLAN_ID,
    );
    expect(screen.getByTestId('graph-exists')).toHaveTextContent('yes');
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it('detects single environment and sets it as active', async () => {
    mockUseParams.mockReturnValue({ workplanId: 'wp-prod' });

    const mockFetch = jest.mocked(global.fetch);
    mockFetch.mockImplementation(async (url: RequestInfo | URL) => {
      if (url.toString().includes('turboci.pa.googleapis.com')) {
        const responseData = ReadWorkPlanResponse.create({
          workplan: {
            identifier: { id: 'wp-prod' },
            checks: [],
            stages: [],
          },
        });
        const binary = ReadWorkPlanResponse.encode(responseData).finish();
        return {
          ok: true,
          arrayBuffer: async () => binary.buffer,
        } as unknown as Response;
      }
      return { ok: false } as unknown as Response;
    });

    render(
      <FakeContextProvider>
        <ChronicleContextProvider>
          <TestWrapper>
            <TestComponent />
          </TestWrapper>
        </ChronicleContextProvider>
      </FakeContextProvider>,
    );

    expect(
      screen.getByText(
        'Detecting the Turbo CI instance that contains workplan wp-prod.',
      ),
    ).toBeInTheDocument();

    await waitFor(() => {
      expect(
        screen.queryByText(
          'Detecting the Turbo CI instance that contains workplan wp-prod.',
        ),
      ).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('active-env')).toHaveTextContent('prod');
  });

  it('defaults to Prod when found in multiple environments including Prod', async () => {
    mockUseParams.mockReturnValue({ workplanId: 'wp-both' });

    const mockFetch = jest.mocked(global.fetch);
    mockFetch.mockImplementation(async () => {
      const responseData = ReadWorkPlanResponse.create({
        workplan: {
          identifier: { id: 'wp-both' },
          checks: [],
          stages: [],
        },
      });
      const binary = ReadWorkPlanResponse.encode(responseData).finish();
      return {
        ok: true,
        arrayBuffer: async () => binary.buffer,
      } as unknown as Response;
    });

    render(
      <FakeContextProvider>
        <ChronicleContextProvider>
          <TestWrapper>
            <TestComponent />
          </TestWrapper>
        </ChronicleContextProvider>
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(
        screen.queryByText(
          'Detecting the Turbo CI instance that contains workplan wp-both.',
        ),
      ).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('active-env')).toHaveTextContent('prod');
  });

  it('displays error message when not found in any environment', async () => {
    mockUseParams.mockReturnValue({ workplanId: 'wp-none' });

    const mockFetch = jest.mocked(global.fetch);
    mockFetch.mockResolvedValue({ ok: false } as unknown as Response);

    render(
      <FakeContextProvider>
        <ChronicleContextProvider>
          <TestWrapper>
            <TestComponent />
          </TestWrapper>
        </ChronicleContextProvider>
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(
        screen.getByText(
          'Workplan wp-none could not be found in any of the Turbo CI environments.',
        ),
      ).toBeInTheDocument();
    });

    expect(screen.queryByTestId('active-env')).not.toBeInTheDocument();
  });

  it('reports timed-out and failed environments correctly', async () => {
    mockUseParams.mockReturnValue({ workplanId: 'wp-timeout' });

    const mockFetch = jest.mocked(global.fetch);
    mockFetch.mockImplementation(async (url: RequestInfo | URL) => {
      if (
        url.toString().includes('qual-staging-turboci.sandbox.googleapis.com')
      ) {
        throw new DOMException('The user aborted a request.', 'AbortError');
      }
      if (url.toString().includes('qual-qa-turboci.sandbox.googleapis.com')) {
        throw new Error('Network error');
      }
      if (url.toString().includes('turboci.pa.googleapis.com')) {
        const responseData = ReadWorkPlanResponse.create({
          workplan: {
            identifier: { id: 'wp-timeout' },
            checks: [],
            stages: [],
          },
        });
        const binary = ReadWorkPlanResponse.encode(responseData).finish();
        return {
          ok: true,
          arrayBuffer: async () => binary.buffer,
        } as unknown as Response;
      }
      return { ok: false } as unknown as Response;
    });

    render(
      <FakeContextProvider>
        <ChronicleContextProvider>
          <TestWrapper>
            <TestComponent />
          </TestWrapper>
        </ChronicleContextProvider>
      </FakeContextProvider>,
    );

    await waitFor(() => {
      expect(
        screen.queryByText(
          'Detecting the Turbo CI instance that contains workplan wp-timeout.',
        ),
      ).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('active-env')).toHaveTextContent('prod');

    expect(screen.getByTestId('failed-envs')).toHaveTextContent(
      'qual-qa:error,qual-staging:timeout',
    );
  });
});
