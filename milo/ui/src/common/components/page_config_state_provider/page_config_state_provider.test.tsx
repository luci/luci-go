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

import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from '@testing-library/react';
import { ErrorBoundary } from 'react-error-boundary';

import {
  PageConfigStateProvider,
  usePageSpecificConfig,
  useSetShowPageConfig,
} from './page_config_state_provider';

function TestConfigButton() {
  const setShowDialog = useSetShowPageConfig();
  return (
    <button
      onClick={() => setShowDialog!(true)}
      disabled={setShowDialog === null}
      data-testid="open-config-dialog"
    >
      show dialog
    </button>
  );
}

function TestPage() {
  const [showDialog, setShowDialog] = usePageSpecificConfig();

  return (
    <>
      {showDialog && <div data-testid="config-dialog">config dialog</div>}
      <button
        onClick={() => setShowDialog(false)}
        data-testid="close-config-dialog"
      >
        close dialog
      </button>
    </>
  );
}

interface TestErrorProps {
  readonly error: unknown;
}

function TestError({ error }: TestErrorProps) {
  return <div data-testid="error">{`${error}`}</div>;
}

describe('PageConfigStateProvider', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    jest.useRealTimers();
  });

  it('e2e', async () => {
    render(
      <PageConfigStateProvider>
        <TestConfigButton />
        <TestPage />
      </PageConfigStateProvider>,
    );

    expect(screen.getByTestId('open-config-dialog')).not.toBeDisabled();
    expect(screen.queryByTestId('config-dialog')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('open-config-dialog'));
    await act(() => jest.runAllTimersAsync());

    expect(screen.queryByTestId('config-dialog')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('close-config-dialog'));
    await act(() => jest.runAllTimersAsync());

    expect(screen.queryByTestId('config-dialog')).not.toBeInTheDocument();
  });

  it('no page with page-specific settings', () => {
    render(
      <PageConfigStateProvider>
        <TestConfigButton />
      </PageConfigStateProvider>,
    );

    expect(screen.getByTestId('open-config-dialog')).toBeDisabled();
  });

  it('multiple pages with page-specific settings', async () => {
    render(
      <PageConfigStateProvider>
        <TestConfigButton />
        <ErrorBoundary FallbackComponent={TestError}>
          <TestPage />
          <TestPage />
        </ErrorBoundary>
      </PageConfigStateProvider>,
    );
    await act(() => jest.runAllTimersAsync());

    expect(screen.getByTestId('error')).toBeInTheDocument();
    expect(screen.getByTestId('error')).toHaveTextContent(
      'Error: cannot support two page config dialogs at the same time',
    );
  });

  it('switch between pages', async () => {
    const { rerender } = render(
      <PageConfigStateProvider>
        <TestConfigButton />
        <TestPage />
      </PageConfigStateProvider>,
    );

    expect(screen.getByTestId('open-config-dialog')).not.toBeDisabled();

    rerender(
      <PageConfigStateProvider>
        <TestConfigButton />
      </PageConfigStateProvider>,
    );
    expect(screen.getByTestId('open-config-dialog')).toBeDisabled();

    rerender(
      <PageConfigStateProvider>
        <TestConfigButton />
        <TestPage />
      </PageConfigStateProvider>,
    );

    expect(screen.getByTestId('open-config-dialog')).not.toBeDisabled();
  });
});
