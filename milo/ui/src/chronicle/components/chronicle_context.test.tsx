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

import { act, render, screen, waitFor } from '@testing-library/react';
import { ReactNode, useContext } from 'react';
import { useParams } from 'react-router';

import { ReadWorkPlanResponse } from '@/proto/turboci/graph/orchestrator/v1/read_workplan_response.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  ChronicleContext,
  DEMO_ENVIRONMENT_NAME,
  DEMO_WORKPLAN_ID,
} from './context';
import { EnvironmentSelectorDialog } from './environment_selector_dialog';
import { ChronicleContextProvider } from './provider';

jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: jest.fn(),
}));

const mockUseParams = jest.mocked(useParams);

describe('ChronicleContextProvider', () => {
  beforeEach(() => {
    global.fetch = jest.fn() as unknown as typeof fetch;
    mockUseParams.mockReturnValue({ workplanId: DEMO_WORKPLAN_ID });
  });

  afterEach(() => {
    jest.resetAllMocks();
  });

  function TestWrapper({ children }: { children: ReactNode }) {
    const {
      workplanId,
      detecting,
      setDetecting,
      detectionFailed,
      showEnvDialog,
      setShowEnvDialog,
      detectedEnvironments,
      setActiveEnvironment,
      requestedEnvFailed,
      failedEnvironments,
      detectionCancelled,
      setDetectionCancelled,
    } = useContext(ChronicleContext);

    const handleDialogClose = () => {
      setDetectionCancelled(true);
      setShowEnvDialog(false);
      setDetecting(false);
    };

    if (detecting) {
      return (
        <div>
          Detecting the Turbo CI instance that contains workplan {workplanId}.
        </div>
      );
    }

    if (detectionCancelled) {
      return (
        <div>
          Selection Cancelled
          <p>
            Environment selection was cancelled. To view the workplan, you must
            select an environment.
          </p>
          <button
            onClick={() => {
              setShowEnvDialog(true);
              setDetectionCancelled(false);
            }}
          >
            Select Environment
          </button>
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

    return (
      <>
        {children}
        {showEnvDialog && (
          <EnvironmentSelectorDialog
            open={showEnvDialog}
            detectedEnvironments={detectedEnvironments}
            requestedEnvFailed={requestedEnvFailed}
            failedEnvironments={failedEnvironments}
            onSelect={(environment) => {
              setActiveEnvironment(environment);
              setShowEnvDialog(false);
              setDetecting(false);
            }}
            onClose={handleDialogClose}
          />
        )}
      </>
    );
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
          <TestComponent />
        </ChronicleContextProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByTestId('workplan-id')).toHaveTextContent(
      DEMO_WORKPLAN_ID,
    );
    expect(screen.getByTestId('active-env')).toHaveTextContent(
      DEMO_ENVIRONMENT_NAME,
    );
    expect(screen.getByTestId('graph-exists')).toHaveTextContent('yes');
    expect(global.fetch).not.toHaveBeenCalled();
  });

  it('detects single environment and sets it as active', async () => {
    mockUseParams.mockReturnValue({ workplanId: 'wp-prod' });

    // Mock fetch responses:
    // PROD succeeds (returns valid graph for wp-prod)
    // Staging fails
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

    // It starts with detecting screen
    expect(
      screen.getByText(
        'Detecting the Turbo CI instance that contains workplan wp-prod.',
      ),
    ).toBeInTheDocument();

    // After detection completes, active environment should be PROD
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

    // All environments return success (including Prod)
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

    // Active env should be Prod, without showing selector dialog
    await waitFor(() => {
      expect(
        screen.queryByText(
          'Detecting the Turbo CI instance that contains workplan wp-both.',
        ),
      ).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('active-env')).toHaveTextContent('prod');
  });

  it('presents environment selector dialog when found in multiple environments not including Prod', async () => {
    mockUseParams.mockReturnValue({ workplanId: 'wp-both' });

    // Mock fetch: Prod fails, Staging & Qual-QA succeed
    const mockFetch = jest.mocked(global.fetch);
    mockFetch.mockImplementation(async (url: RequestInfo | URL) => {
      if (url.toString().includes('turboci.pa.googleapis.com')) {
        return { ok: false } as unknown as Response;
      }
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

    // Wait for the selector dialog to appear
    await waitFor(() => {
      expect(
        screen.getByText('Select Orchestrator Environment'),
      ).toBeInTheDocument();
    });

    // Select qual-staging
    const stagingBtn = screen.getByText('qual-staging');
    act(() => {
      stagingBtn.click();
    });

    // Dialog should close, and active env should be Staging
    await waitFor(() => {
      expect(
        screen.queryByText('Select Orchestrator Environment'),
      ).not.toBeInTheDocument();
    });
    expect(screen.getByTestId('active-env')).toHaveTextContent('qual-staging');
  });

  it('displays error message when not found in any environment', async () => {
    mockUseParams.mockReturnValue({ workplanId: 'wp-none' });

    // Both environments return 404
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

  it('uses URL env parameter if it exists and workplan is found there', async () => {
    mockUseParams.mockReturnValue({ workplanId: 'wp-staging' });

    // Mock fetch: Staging succeeds, Prod succeeds
    const mockFetch = jest.mocked(global.fetch);
    mockFetch.mockImplementation(async (url: RequestInfo | URL) => {
      if (
        url
          .toString()
          .includes('qual-staging-turboci.sandbox.googleapis.com') ||
        url.toString().includes('turboci.pa.googleapis.com')
      ) {
        const responseData = ReadWorkPlanResponse.create({
          workplan: {
            identifier: { id: 'wp-staging' },
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
      <FakeContextProvider
        routerOptions={{
          initialEntries: ['/?env=qual-staging'],
        }}
      >
        <ChronicleContextProvider>
          <TestWrapper>
            <TestComponent />
          </TestWrapper>
        </ChronicleContextProvider>
      </FakeContextProvider>,
    );

    // Active env should be set to Staging (requested in URL)
    await waitFor(() => {
      expect(
        screen.queryByText(
          'Detecting the Turbo CI instance that contains workplan wp-staging.',
        ),
      ).not.toBeInTheDocument();
    });

    expect(
      screen.queryByText('Select Orchestrator Environment'),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('active-env')).toHaveTextContent('qual-staging');
  });

  it('shows dialog with failure warning when URL env is specified but workplan is NOT found there', async () => {
    mockUseParams.mockReturnValue({ workplanId: 'wp-prod' });

    // Mock fetch: Staging fails, Prod succeeds
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
      <FakeContextProvider
        routerOptions={{
          initialEntries: ['/?env=qual-staging'],
        }}
      >
        <ChronicleContextProvider>
          <TestWrapper>
            <TestComponent />
          </TestWrapper>
        </ChronicleContextProvider>
      </FakeContextProvider>,
    );

    // Selector dialog should appear and show that it failed in Staging
    await waitFor(() => {
      expect(
        screen.getByText('Select Orchestrator Environment'),
      ).toBeInTheDocument();
    });

    expect(
      screen.getByText(
        'Workplan was not found in the requested environment: qual-staging.',
      ),
    ).toBeInTheDocument();

    expect(screen.getByTestId('active-env')).toHaveTextContent('');
  });

  it('handles dialog cancellation correctly by showing cancelled screen', async () => {
    mockUseParams.mockReturnValue({ workplanId: 'wp-both' });

    // Mock fetch: Prod fails, Staging & Qual-QA succeed
    const mockFetch = jest.mocked(global.fetch);
    mockFetch.mockImplementation(async (url: RequestInfo | URL) => {
      if (url.toString().includes('turboci.pa.googleapis.com')) {
        return { ok: false } as unknown as Response;
      }
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

    // Wait for the selector dialog to appear
    await waitFor(() => {
      expect(
        screen.getByText('Select Orchestrator Environment'),
      ).toBeInTheDocument();
    });

    // Cancel the dialog
    const cancelBtn = screen.getByText('Cancel');
    act(() => {
      cancelBtn.click();
    });

    // It should close and show "Selection Cancelled" screen
    await waitFor(() => {
      expect(
        screen.queryByText('Select Orchestrator Environment'),
      ).not.toBeInTheDocument();
    });

    expect(screen.getByText('Selection Cancelled')).toBeInTheDocument();
    expect(
      screen.getByText(
        'Environment selection was cancelled. To view the workplan, you must select an environment.',
      ),
    ).toBeInTheDocument();

    // Clicking "Select Environment" should reopen the dialog
    const selectBtn = screen.getByText('Select Environment');
    act(() => {
      selectBtn.click();
    });

    await waitFor(() => {
      expect(
        screen.getByText('Select Orchestrator Environment'),
      ).toBeInTheDocument();
    });
  });

  it('reports timed-out and failed environments correctly', async () => {
    mockUseParams.mockReturnValue({ workplanId: 'wp-timeout' });

    // Mock fetch: Staging times out (throws AbortError), Qual-QA fails (throws network error), Prod succeeds
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

    // Wait for detection to finish. It should resolve to Prod (since Prod succeeded and is default)
    await waitFor(() => {
      expect(
        screen.queryByText(
          'Detecting the Turbo CI instance that contains workplan wp-timeout.',
        ),
      ).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('active-env')).toHaveTextContent('prod');

    // It should report failed/timed-out environments
    expect(screen.getByTestId('failed-envs')).toHaveTextContent(
      'qual-qa:error,qual-staging:timeout',
    );
  });
});
