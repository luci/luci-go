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

import { fireEvent, render, screen } from '@testing-library/react';
import { act } from 'react';

import { buildStructuredAlerts } from '@/monitoringv2/util/alerts';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  BuilderAlertBuilder,
  StepAlertBuilder,
} from '../testing_tools/test_utils';

import { AlertTable } from './alert_table';

describe('<AlertTable />', () => {
  it('displays an alert', async () => {
    const b = new BuilderAlertBuilder().withBuilder('linux-rel').build();
    const alerts = buildStructuredAlerts([b]);

    render(
      <FakeContextProvider>
        <AlertTable alerts={alerts} groups={[]} selectedTab="" />
      </FakeContextProvider>,
    );

    expect(screen.getByText('linux-rel')).toBeInTheDocument();
  });

  it('expands an alert on click', async () => {
    const b = new BuilderAlertBuilder().withBuilder('linux-rel').build();
    const s = new StepAlertBuilder().withBuilder('linux-rel').build();
    const alerts = buildStructuredAlerts([b, s]);
    const builderAlerts = alerts.filter((a) => a.alert.kind === 'builder');

    render(
      <FakeContextProvider>
        <AlertTable alerts={builderAlerts} groups={[]} selectedTab="" />
      </FakeContextProvider>,
    );
    expect(screen.getByText('linux-rel')).toBeInTheDocument();
    expect(screen.queryByText('Step:')).toBeNull();
    await act(() => fireEvent.click(screen.getByTitle('Expand')));

    expect(screen.getByText('Step:')).toBeInTheDocument();
  });

  it('sorts alerts on header click', async () => {
    const b1 = new BuilderAlertBuilder().withBuilder('linux-rel').build();
    const b2 = new BuilderAlertBuilder().withBuilder('win-rel').build();
    const alerts = buildStructuredAlerts([b1, b2]);

    render(
      <FakeContextProvider>
        <AlertTable alerts={alerts} groups={[]} selectedTab="" />
      </FakeContextProvider>,
    );
    expect(screen.getByText('linux-rel')).toBeInTheDocument();
    expect(screen.getByText('win-rel')).toBeInTheDocument();

    // Sort asc
    await act(() => fireEvent.click(screen.getByText('Failure')));
    let linux = screen.getByText('linux-rel');
    let win = screen.getByText('win-rel');
    expect(linux.compareDocumentPosition(win)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );

    // Sort desc
    await act(() => fireEvent.click(screen.getByText('Failure')));
    linux = screen.getByText('linux-rel');
    win = screen.getByText('win-rel');
    expect(linux.compareDocumentPosition(win)).toBe(
      Node.DOCUMENT_POSITION_PRECEDING,
    );
  });
});
