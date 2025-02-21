// Copyright 2025 The LUCI Authors.
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

import { CountDevicesResponse } from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { MainMetrics } from './main_metrics';

const MOCK_DATA: CountDevicesResponse = {
  total: 440,

  taskState: {
    busy: 401,
    idle: 40,
  },
  deviceState: {
    ready: 40,
    needManualRepair: 40,
    needRepair: 40,
    repairFailed: 40,
  },
};

describe('<MainMetrics />', () => {
  it('should render', async () => {
    const mockCountQuery = jest.fn().mockReturnValue({
      data: MOCK_DATA,
      isLoading: false,
      error: null,
    });

    render(
      <FakeContextProvider>
        <MainMetrics countQuery={mockCountQuery()} />
      </FakeContextProvider>,
    );

    const total = screen.getByText('Total');
    expect(total).toBeVisible();
  });
});
