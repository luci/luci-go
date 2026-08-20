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

import { act, renderHook, waitFor } from '@testing-library/react';

import * as PrpcClientsModule from '@/fleet/hooks/prpc_clients';
import {
  CreatePriorityRuleRequest,
  ListPriorityRulesResponse,
  PriorityRule,
  UpdatePriorityRuleRequest,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  PRIORITY_RULES_QUERY_KEY,
  usePriorityRules,
} from './use_priority_rules';

const INITIAL_RULES: readonly PriorityRule[] = [
  {
    id: '1',
    expressionAip160: 'board = "bria"',
    weight: '100',
  },
  {
    id: '2',
    expressionAip160: 'pool = "quota"',
    weight: '200',
  },
];

describe('usePriorityRules', () => {
  let mockListPriorityRules: jest.Mock;
  let mockCreatePriorityRule: jest.Mock;
  let mockUpdatePriorityRule: jest.Mock;
  let mockDeletePriorityRule: jest.Mock;

  beforeEach(() => {
    jest.clearAllMocks();

    mockListPriorityRules = jest.fn().mockReturnValue({
      queryKey: PRIORITY_RULES_QUERY_KEY,
      queryFn: async (): Promise<ListPriorityRulesResponse> => ({
        priorityRules: INITIAL_RULES,
      }),
    });

    mockCreatePriorityRule = jest
      .fn()
      .mockImplementation(async (req: CreatePriorityRuleRequest) => ({
        priorityRule: {
          id: '3',
          expressionAip160: req.priorityRule?.expressionAip160 ?? '',
          weight: req.priorityRule?.weight ?? '0',
        },
      }));

    mockUpdatePriorityRule = jest
      .fn()
      .mockImplementation(async (req: UpdatePriorityRuleRequest) => ({
        priorityRule: {
          id: req.id,
          expressionAip160: req.expressionAip160 ?? '',
          weight: req.weight ?? '0',
        },
      }));

    mockDeletePriorityRule = jest.fn().mockResolvedValue({});

    jest.spyOn(PrpcClientsModule, 'useFleetConsoleClient').mockReturnValue({
      ListPriorityRules: {
        query: mockListPriorityRules,
      },
      CreatePriorityRule: mockCreatePriorityRule,
      UpdatePriorityRule: mockUpdatePriorityRule,
      DeletePriorityRule: mockDeletePriorityRule,
    } as unknown as ReturnType<typeof PrpcClientsModule.useFleetConsoleClient>);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('fetches and returns initial priority rules list', async () => {
    const { result } = renderHook(() => usePriorityRules(), {
      wrapper: FakeContextProvider,
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.rules).toEqual(INITIAL_RULES);
  });

  it('creates rule and calls CreatePriorityRule RPC on createRule', async () => {
    const { result } = renderHook(() => usePriorityRules(), {
      wrapper: FakeContextProvider,
    });

    await waitFor(() => {
      expect(result.current.rules.length).toBe(2);
    });

    let createdRes!: { readonly priorityRule?: PriorityRule };
    await act(async () => {
      createdRes = await result.current.createRule({
        priorityRule: {
          id: '0',
          expressionAip160: 'model = "volteer"',
          weight: '500',
        },
      });
    });

    expect(mockCreatePriorityRule).toHaveBeenCalledWith({
      priorityRule: {
        id: '0',
        expressionAip160: 'model = "volteer"',
        weight: '500',
      },
    });
    expect(createdRes.priorityRule?.id).toBe('3');
    expect(createdRes.priorityRule?.expressionAip160).toBe('model = "volteer"');
  });

  it('propagates error when createRule fails', async () => {
    mockCreatePriorityRule.mockRejectedValueOnce(new Error('Backend error'));

    const { result } = renderHook(() => usePriorityRules(), {
      wrapper: FakeContextProvider,
    });

    await waitFor(() => {
      expect(result.current.rules.length).toBe(2);
    });

    let caughtError: Error | undefined;
    await act(async () => {
      try {
        await result.current.createRule({
          priorityRule: {
            id: '0',
            expressionAip160: 'model = "volteer"',
            weight: '500',
          },
        });
      } catch (err) {
        if (err instanceof Error) {
          caughtError = err;
        }
      }
    });

    expect(caughtError?.message).toBe('Backend error');
    // Rules remain unchanged
    expect(result.current.rules.length).toBe(2);
  });

  it('optimistically updates rule on updateRule and rolls back on error', async () => {
    let rejectUpdate!: (err: Error) => void;
    mockUpdatePriorityRule.mockReturnValue(
      new Promise((_, reject) => {
        rejectUpdate = reject;
      }),
    );

    const { result } = renderHook(() => usePriorityRules(), {
      wrapper: FakeContextProvider,
    });

    await waitFor(() => {
      expect(result.current.rules.length).toBe(2);
    });

    let caughtError: Error | undefined;
    act(() => {
      result.current
        .updateRule({
          id: '1',
          weight: '999',
        })
        .catch((err: unknown) => {
          if (err instanceof Error) {
            caughtError = err;
          }
        });
    });

    // Optimistically updated
    await waitFor(() => {
      const r1 = result.current.rules.find((r) => r.id === '1');
      expect(r1?.weight).toBe('999');
      expect(r1?.expressionAip160).toBe('board = "bria"');
    });

    // Reject update
    await act(async () => {
      rejectUpdate(new Error('Update failed'));
    });

    await waitFor(() => {
      expect(caughtError?.message).toBe('Update failed');
    });

    // Rolled back
    await waitFor(() => {
      const r1 = result.current.rules.find((r) => r.id === '1');
      expect(r1?.weight).toBe('100');
    });
  });

  it('optimistically removes rule on deleteRule and rolls back on error', async () => {
    let rejectDelete!: (err: Error) => void;
    mockDeletePriorityRule.mockReturnValue(
      new Promise((_, reject) => {
        rejectDelete = reject;
      }),
    );

    const { result } = renderHook(() => usePriorityRules(), {
      wrapper: FakeContextProvider,
    });

    await waitFor(() => {
      expect(result.current.rules.length).toBe(2);
    });

    let caughtError: Error | undefined;
    act(() => {
      result.current.deleteRule({ id: '1' }).catch((err: unknown) => {
        if (err instanceof Error) {
          caughtError = err;
        }
      });
    });

    // Optimistically removed
    await waitFor(() => {
      expect(result.current.rules.length).toBe(1);
      expect(result.current.rules.find((r) => r.id === '1')).toBeUndefined();
    });

    // Reject delete
    await act(async () => {
      rejectDelete(new Error('Delete forbidden'));
    });

    await waitFor(() => {
      expect(caughtError?.message).toBe('Delete forbidden');
    });

    // Rolled back
    await waitFor(() => {
      expect(result.current.rules.length).toBe(2);
      expect(result.current.rules.find((r) => r.id === '1')).toBeDefined();
    });
  });
});
