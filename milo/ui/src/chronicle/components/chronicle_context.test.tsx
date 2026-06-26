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

import { render, screen } from '@testing-library/react';
import { useContext } from 'react';
import { useParams } from 'react-router';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ChronicleContext, DEMO_WORKPLAN_ID } from './context';
import { ChronicleContextProvider } from './provider';

jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: jest.fn(),
}));

const mockUseParams = jest.mocked(useParams);

describe('ChronicleContextProvider', () => {
  beforeEach(() => {
    mockUseParams.mockReturnValue({ workplanId: DEMO_WORKPLAN_ID });
  });

  afterEach(() => {
    jest.resetAllMocks();
  });

  function TestComponent() {
    const { workplanId, activeEnvironment, graph } =
      useContext(ChronicleContext);
    return (
      <div>
        <div data-testid="workplan-id">{workplanId}</div>
        <div data-testid="active-env">{activeEnvironment}</div>
        <div data-testid="graph-exists">{graph ? 'yes' : 'no'}</div>
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
      DEMO_WORKPLAN_ID,
    );
    expect(screen.getByTestId('graph-exists')).toHaveTextContent('yes');
  });
});
