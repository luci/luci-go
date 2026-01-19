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
import userEvent from '@testing-library/user-event';
import { useSearchParams } from 'react-router';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useArtifactFilters } from './context';
import { ArtifactFilterProvider } from './provider';

function TestComponent() {
  const { hideEmptyFolders, setHideEmptyFolders } = useArtifactFilters();
  const [searchParams] = useSearchParams();

  return (
    <div>
      <div data-testid="hide-value">{hideEmptyFolders.toString()}</div>
      <div data-testid="param-value">
        {searchParams.get('no_empty') ?? 'null'}
      </div>
      <button onClick={() => setHideEmptyFolders(true)}>Hide</button>
      <button onClick={() => setHideEmptyFolders(false)}>Show</button>
    </div>
  );
}

describe('ArtifactFilterProvider', () => {
  it('should sync hideEmptyFolders with no_empty search param correctly', async () => {
    const user = userEvent.setup();
    render(
      <FakeContextProvider>
        <ArtifactFilterProvider>
          <TestComponent />
        </ArtifactFilterProvider>
      </FakeContextProvider>,
    );

    // Initial state: Default is hidden (true), param is null (deleted)
    expect(screen.getByTestId('hide-value')).toHaveTextContent('true');
    expect(screen.getByTestId('param-value')).toHaveTextContent('null');

    // User clicks "Show" (hide=false) -> param should become 'false'
    await user.click(screen.getByText('Show'));
    expect(screen.getByTestId('param-value')).toHaveTextContent('false');
    expect(screen.getByTestId('hide-value')).toHaveTextContent('false');

    // User clicks "Hide" (hide=true) -> param should be deleted (null)
    await user.click(screen.getByText('Hide'));
    expect(screen.getByTestId('param-value')).toHaveTextContent('null');
    expect(screen.getByTestId('hide-value')).toHaveTextContent('true');
  });
});
