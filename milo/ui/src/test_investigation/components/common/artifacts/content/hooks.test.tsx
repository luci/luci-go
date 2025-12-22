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

import { renderHook, act } from '@testing-library/react';

import { CompareArtifactLinesResponse_FailureOnlyRange } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

import { useArtifactViewItems } from './hooks';

describe('useArtifactItems', () => {
  it('should maintain divider and correct handlers when expanding lines', () => {
    const lines = Array.from({ length: 100 }, (_, i) => `Line ${i}`);
    const ranges: CompareArtifactLinesResponse_FailureOnlyRange[] = [
      CompareArtifactLinesResponse_FailureOnlyRange.fromPartial({
        startLine: 50,
        endLine: 60,
      }),
    ];
    const lineMatches = new Map();

    const { result } = renderHook(() =>
      useArtifactViewItems(lines, ranges, false, lineMatches),
    );

    let items = result.current;
    let divider0 = items.find((i) => i.type === 'divider');
    expect(divider0).toBeDefined();
    expect(divider0!.numLines).toBe(50);
    expect(divider0!.onExpandStart).toBeUndefined();
    expect(divider0!.onExpandEnd).toBeDefined();

    act(() => {
      divider0!.onExpandEnd!(10);
    });

    items = result.current;
    const visibleLines = items.filter(
      (i) => i.type === 'line' && i.originalLineIndex! < 50,
    );
    expect(visibleLines.length).toBe(10);

    divider0 = items.find((i) => i.type === 'divider');
    expect(divider0).toBeDefined();
    expect(divider0!.numLines).toBe(40);

    let divider1 = items.filter((i) => i.type === 'divider')[1];
    expect(divider1).toBeDefined();
    expect(divider1!.numLines).toBe(40);
    expect(divider1!.onExpandStart).toBeDefined();
    expect(divider1!.onExpandEnd).toBeUndefined();

    act(() => {
      divider1.onExpandStart!(10);
    });

    items = result.current;
    divider1 = items.filter((i) => i.type === 'divider')[1];
    expect(divider1.numLines).toBe(30);
  });

  it('should reproduce the bug: divider disappears when small', () => {
    const lines = Array.from({ length: 100 }, (_, i) => `Line ${i}`);

    const ranges: CompareArtifactLinesResponse_FailureOnlyRange[] = [
      CompareArtifactLinesResponse_FailureOnlyRange.fromPartial({
        startLine: 5,
        endLine: 10,
      }),
      CompareArtifactLinesResponse_FailureOnlyRange.fromPartial({
        startLine: 23,
        endLine: 30,
      }),
    ];

    const { result } = renderHook(() =>
      useArtifactViewItems(lines, ranges, false, new Map()),
    );

    let items = result.current;
    const divider = items
      .filter((i) => i.type === 'divider')
      .find((i) => i.numLines === 13);
    expect(divider).toBeDefined();
    expect(divider!.onExpandStart).toBeDefined();
    expect(divider!.onExpandEnd).toBeDefined();

    act(() => {
      divider!.onExpandStart!(10);
    });

    items = result.current;

    const dividers = items.filter((i) => i.type === 'divider');
    expect(dividers.length).toBe(3);
    const targetDivider = dividers[1];

    expect(targetDivider).toBeDefined();
    expect(targetDivider.numLines).toBe(3);
    expect(targetDivider.onExpandStart).toBeDefined();
    expect(targetDivider.onExpandEnd).toBeDefined();
  });
});
