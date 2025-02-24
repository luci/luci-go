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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, renderHook } from '@testing-library/react';
import { act } from 'react';

import { deferred } from '@/generic_libs/tools/utils';

import { Lexer } from './lexer';
import { useSuggestions } from './suggestion';
import { getSuggestionCtx } from './suggestion';
import { FieldDef } from './types';

describe('useSuggestions', () => {
  const queryClient = new QueryClient();
  const Wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );

  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    cleanup();
  });

  it('should suggest fields', () => {
    const schema: FieldDef = {
      fields: {
        project: {},
        projectWithSuffix: {},
        builder: {},
      },
    };

    const { result } = renderHook(() => useSuggestions(schema, 'projec', 3), {
      wrapper: Wrapper,
    });
    expect(result.current).toEqual([
      {
        id: 'project',
        value: {
          text: 'project',
          display: expect.anything(),
          apply: expect.any(Function),
        },
      },
      {
        id: 'projectWithSuffix',
        value: {
          text: 'projectWithSuffix',
          display: expect.anything(),
          apply: expect.any(Function),
        },
      },
    ]);
    expect(result.current[0]!.value.apply()).toEqual(['project', 7]);
    expect(result.current[1]!.value.apply()).toEqual(['projectWithSuffix', 17]);
  });

  it('should suggest nested fields', () => {
    const schema: FieldDef = {
      fields: {
        build: {
          fields: {
            builderId: {
              fields: {
                project: {},
              },
            },
            // Should not be suggested when `builderId.` prefix exists.
            projectWithSuffix: {},
          },
        },
      },
    };

    const { result } = renderHook(
      () => useSuggestions(schema, 'build.builderId.pro', 3),
      {
        wrapper: Wrapper,
      },
    );
    expect(result.current).toEqual([
      {
        id: 'project',
        value: {
          text: 'project',
          display: expect.anything(),
          apply: expect.any(Function),
        },
      },
    ]);
    expect(result.current[0]!.value.apply()).toEqual([
      'build.builderId.project',
      23,
    ]);
  });

  it('should not suggest anything when the parent field does not exist', () => {
    const schema: FieldDef = {
      fields: {
        builderId: {
          fields: {
            project: {},
          },
        },
        projectWithSuffix: {},
      },
    };

    const { result } = renderHook(
      () => useSuggestions(schema, 'builder.pro', 3),
      {
        wrapper: Wrapper,
      },
    );
    expect(result.current).toEqual([]);
  });

  it('should be case insensitive when suggesting fields', () => {
    const schema: FieldDef = {
      fields: {
        Project: {},
        project: {},
      },
    };

    const { result } = renderHook(() => useSuggestions(schema, 'project', 3), {
      wrapper: Wrapper,
    });
    expect(result.current).toEqual([
      {
        id: 'Project',
        value: {
          text: 'Project',
          display: expect.anything(),
          apply: expect.any(Function),
        },
      },
    ]);
  });

  it('should not suggest field with the exact match', () => {
    const schema: FieldDef = {
      fields: {
        project: {},
        projectWithSuffix: {},
      },
    };

    const { result } = renderHook(() => useSuggestions(schema, 'project', 3), {
      wrapper: Wrapper,
    });
    expect(result.current).toEqual([
      {
        id: 'projectWithSuffix',
        value: {
          text: 'projectWithSuffix',
          display: expect.anything(),
          apply: expect.any(Function),
        },
      },
    ]);
  });

  it('should suggest values', () => {
    const schema: FieldDef = {
      fields: {
        project: {
          getValues: (partial) =>
            ['chromium', 'chromeos']
              .filter((text) => text.includes(partial))
              .map((text) => ({ text })),
        },
      },
    };

    const { result } = renderHook(
      () => useSuggestions(schema, 'project = chrom', 13),
      { wrapper: Wrapper },
    );
    expect(result.current).toEqual([
      {
        id: 'chromium',
        value: {
          text: 'chromium',
          apply: expect.any(Function),
        },
      },
      {
        id: 'chromeos',
        value: {
          text: 'chromeos',
          apply: expect.any(Function),
        },
      },
    ]);
    expect(result.current[0]!.value.apply()).toEqual([
      'project = chromium',
      18,
    ]);
    expect(result.current[1]!.value.apply()).toEqual([
      'project = chromeos',
      18,
    ]);
  });

  it('should suggest value of a nested field', () => {
    const schema: FieldDef = {
      fields: {
        build: {
          fields: {
            builderId: {
              fields: {
                project: {
                  getValues: (partial) =>
                    ['chromium', 'chromeos']
                      .filter((text) => text.includes(partial))
                      .map((text) => ({ text })),
                },
              },
            },
          },
        },
      },
    };

    const { result } = renderHook(
      () => useSuggestions(schema, 'build.builderId.project = chrom', 28),
      { wrapper: Wrapper },
    );
    expect(result.current).toEqual([
      {
        id: 'chromium',
        value: {
          text: 'chromium',
          apply: expect.any(Function),
        },
      },
      {
        id: 'chromeos',
        value: {
          text: 'chromeos',
          apply: expect.any(Function),
        },
      },
    ]);
    expect(result.current[0]!.value.apply()).toEqual([
      'build.builderId.project = chromium',
      34,
    ]);
    expect(result.current[1]!.value.apply()).toEqual([
      'build.builderId.project = chromeos',
      34,
    ]);
  });

  it('should suggest values asynchronously', async () => {
    const [blocker, resolveBlocker] = deferred();
    const schema: FieldDef = {
      fields: {
        project: {
          fetchValues: (partial: string) => ({
            queryKey: [partial],
            queryFn: async () => {
              await blocker;
              return ['chromium', 'chromeos']
                .filter((text) => text.includes(partial) && text !== partial)
                .map((text) => ({ text }));
            },
          }),
        },
      },
    };

    const { result } = renderHook(
      () => useSuggestions(schema, 'project = chromi', 12),
      { wrapper: Wrapper },
    );

    // Initially, should show loading.
    expect(result.current).toEqual([
      {
        id: 'loading',
        value: {
          text: 'Loading...',
          display: expect.anything(),
          apply: expect.any(Function),
        },
        unselectable: true,
      },
    ]);

    resolveBlocker();
    await act(() => jest.runOnlyPendingTimersAsync());

    expect(result.current).toEqual([
      {
        id: 'chromium',
        value: {
          text: 'chromium',
          apply: expect.any(Function),
        },
      },
    ]);
    expect(result.current[0]!.value.apply()).toEqual([
      'project = chromium',
      18,
    ]);
  });
});

describe('getSuggestionCtx', () => {
  it.each([
    // Cursor on field.
    ['|project'],
    ['pro|ject'],
    ['project|'],

    // Cursor on value.
    ['project = |chromium'],
    ['project = chro|mium'],
    ['project = chromium|'],

    // Cursor on value no whitespace.
    ['project=|chromium'],
    ['project=chro|mium'],
    ['project=chromium|'],

    // Cursor after value.
    ['project=chromium |'],

    // Cursor after comparator.
    ['project = | '],

    // Cursor after comparator with longer whitespace.
    ['project =   | '],

    // Cursor on field with following value.
    ['|project=chromium'],
    ['pro|ject=chromium'],
    ['project|=chromium'],

    // Cursor on whitespace after field.
    ['project |'],
    ['project | another_project'],

    // Quoted string.
    ['project = "chromi|um"'],
    // Unclosed quoted string.
    ['project = "chromi|um'],
  ])('inputWithCursor: %s', (inputWithCursor) => {
    const input = inputWithCursor.replace('|', '');
    const cursorPos = inputWithCursor.indexOf('|');
    const lexer = new Lexer(input);
    expect(getSuggestionCtx(lexer, cursorPos)).toMatchSnapshot();
  });
});
