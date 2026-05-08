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

import { cleanup, render, screen } from '@testing-library/react';
import { userEvent } from '@testing-library/user-event';

import { FailureReason } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/failure_reason.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { FailureReasonEntry } from './failure_reason_entry';

describe('<FailureReasonEntry />', () => {
  afterEach(() => {
    cleanup();
  });

  it('should render fallback primaryErrorMessage when errors list is empty', () => {
    const failureReason = FailureReason.fromPartial({
      primaryErrorMessage: 'fallback error message',
      errors: [],
    });

    render(
      <FakeContextProvider>
        <FailureReasonEntry failureReason={failureReason} />
      </FakeContextProvider>,
    );

    expect(screen.getByText('fallback error message')).toBeInTheDocument();
    expect(screen.queryByText('Additional Errors')).not.toBeInTheDocument();
  });

  it('should render primary error message and show trace when clicked', async () => {
    const failureReason = FailureReason.fromPartial({
      errors: [
        {
          message: 'primary error message',
          trace: 'primary stack trace',
        },
      ],
    });

    render(
      <FakeContextProvider>
        <FailureReasonEntry failureReason={failureReason} />
      </FakeContextProvider>,
    );

    expect(screen.getByText('primary error message')).toBeInTheDocument();

    // Primary trace is shown by default (isPrimary=true sets showTrace=true)
    expect(screen.getByText('primary stack trace')).toBeInTheDocument();

    // Click to hide
    await userEvent.click(screen.getByText('Stack Trace'));
    expect(screen.getByText('primary stack trace')).not.toBeVisible();
  });

  it('should render additional errors in collapsible section', async () => {
    const failureReason = FailureReason.fromPartial({
      errors: [
        {
          message: 'primary error message',
          trace: 'primary stack trace',
        },
        {
          message: 'second error message',
          trace: 'second stack trace',
        },
        {
          message: 'third error message',
          trace: '',
        },
      ],
    });

    render(
      <FakeContextProvider>
        <FailureReasonEntry failureReason={failureReason} />
      </FakeContextProvider>,
    );

    expect(screen.getByText('primary error message')).toBeInTheDocument();

    // Additional errors should be hidden by default
    expect(screen.getByText('Additional Errors (2 more)')).toBeInTheDocument();
    expect(screen.queryByText('second error message')).not.toBeInTheDocument();

    // Expand additional errors
    await userEvent.click(screen.getByText('Additional Errors (2 more)'));

    expect(screen.getByText('second error message')).toBeInTheDocument();
    expect(screen.getByText('third error message')).toBeInTheDocument();

    // Second error trace should be hidden by default (isPrimary=false)
    expect(screen.queryByText('second stack trace')).not.toBeInTheDocument();

    // Show second error trace
    const showButtons = screen.getAllByText('Stack Trace');
    // Index 0 is for primary error, Index 1 is for second error
    expect(showButtons.length).toBe(2);
    await userEvent.click(showButtons[1]);
    expect(screen.getByText('second stack trace')).toBeInTheDocument();
  });

  it('should render truncation warning when truncatedErrorsCount > 0', async () => {
    const failureReason = FailureReason.fromPartial({
      errors: [{ message: 'primary error' }, { message: 'second error' }],
      truncatedErrorsCount: 5,
    });

    render(
      <FakeContextProvider>
        <FailureReasonEntry failureReason={failureReason} />
      </FakeContextProvider>,
    );

    await userEvent.click(screen.getByText('Additional Errors (6 more)'));

    expect(
      screen.getByText(/5 errors were truncated due to size limits/),
    ).toBeInTheDocument();
  });

  it('should render truncation warning even if there is only one non-truncated error', async () => {
    const failureReason = FailureReason.fromPartial({
      errors: [{ message: 'primary error' }],
      truncatedErrorsCount: 5,
    });

    render(
      <FakeContextProvider>
        <FailureReasonEntry failureReason={failureReason} />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Additional Errors (5 more)')).toBeInTheDocument();

    await userEvent.click(screen.getByText('Additional Errors (5 more)'));

    expect(
      screen.getByText(/5 errors were truncated due to size limits/),
    ).toBeInTheDocument();
  });
});
