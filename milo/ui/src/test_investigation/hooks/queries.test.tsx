import { renderHook, waitFor } from '@testing-library/react';

import { useFeatureFlag } from '@/common/feature_flags/context';
import { useResultDbClient } from '@/common/hooks/prpc_clients';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { RootInvocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/root_invocation.pb';
import { USE_ROOT_INVOCATION_FLAG } from '@/test_investigation/pages/features';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useInvocationQuery } from './queries';

jest.mock('@/common/hooks/prpc_clients');
jest.mock('@/common/feature_flags/context');

const mockUseResultDbClient = jest.mocked(useResultDbClient);
const mockUseFeatureFlag = jest.mocked(useFeatureFlag);

describe('useInvocationQuery', () => {
  let mockGetRootInvocationQuery: jest.Mock;
  let mockGetInvocationQuery: jest.Mock;

  const mockRootInvocation = RootInvocation.fromPartial({
    name: 'rootInvocations/build-1234',
  });

  const mockLegacyInvocation = Invocation.fromPartial({
    name: 'invocations/build-1234',
  });

  beforeEach(() => {
    mockGetRootInvocationQuery = jest.fn();
    mockGetInvocationQuery = jest.fn();

    mockUseResultDbClient.mockReturnValue({
      GetRootInvocation: { query: mockGetRootInvocationQuery },
      GetInvocation: { query: mockGetInvocationQuery },
    } as unknown as ReturnType<typeof useResultDbClient>);

    mockUseFeatureFlag.mockImplementation((flag) => {
      if (flag === USE_ROOT_INVOCATION_FLAG) {
        return false;
      }
      return false;
    });
  });

  afterEach(() => {
    jest.resetAllMocks();
  });

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <FakeContextProvider>{children}</FakeContextProvider>
  );

  it('should fetch Root Invocation first when flag is ON', async () => {
    mockUseFeatureFlag.mockReturnValue(true);

    mockGetRootInvocationQuery.mockReturnValue({
      queryKey: ['rootInv'],
      queryFn: jest.fn().mockResolvedValue(mockRootInvocation),
    });

    mockGetInvocationQuery.mockReturnValue({
      queryKey: ['legacyInv'],
      queryFn: jest.fn(),
    });

    const { result } = renderHook(() => useInvocationQuery('build-1234'), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.invocation?.data).toEqual(mockRootInvocation);
    expect(result.current.invocation?.isLegacyInvocation).toBe(false);
  });

  it('should fallback to Legacy Invocation when flag is ON and Root fails', async () => {
    mockUseFeatureFlag.mockReturnValue(true);

    mockGetRootInvocationQuery.mockReturnValue({
      queryKey: ['rootInv'],
      queryFn: jest.fn().mockRejectedValue(new Error('Root not found')),
      retry: false,
    });

    mockGetInvocationQuery.mockReturnValue({
      queryKey: ['legacyInv'],
      queryFn: jest.fn().mockResolvedValue(mockLegacyInvocation),
      retry: false,
    });

    const { result } = renderHook(() => useInvocationQuery('build-1234'), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.invocation?.data).toEqual(mockLegacyInvocation);
    expect(result.current.invocation?.isLegacyInvocation).toBe(true);
  });

  it('should fetch Legacy Invocation first when flag is OFF', async () => {
    mockUseFeatureFlag.mockReturnValue(false);

    mockGetInvocationQuery.mockReturnValue({
      queryKey: ['legacyInv'],
      queryFn: jest.fn().mockResolvedValue(mockLegacyInvocation),
    });
    mockGetRootInvocationQuery.mockReturnValue({
      queryKey: ['rootInv'],
      queryFn: jest.fn(),
    });

    const { result } = renderHook(() => useInvocationQuery('build-1234'), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.invocation?.data).toEqual(mockLegacyInvocation);
    expect(result.current.invocation?.isLegacyInvocation).toBe(true);
  });

  it('should fallback to Root Invocation when flag is OFF and Legacy fails', async () => {
    mockUseFeatureFlag.mockReturnValue(false);

    mockGetInvocationQuery.mockReturnValue({
      queryKey: ['legacyInv'],
      queryFn: jest.fn().mockRejectedValue(new Error('Legacy not found')),
      retry: false,
    });

    mockGetRootInvocationQuery.mockReturnValue({
      queryKey: ['rootInv'],
      queryFn: jest.fn().mockResolvedValue(mockRootInvocation),
      retry: false,
    });

    const { result } = renderHook(() => useInvocationQuery('build-1234'), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.invocation?.data).toEqual(mockRootInvocation);
    expect(result.current.invocation?.isLegacyInvocation).toBe(false);
  });

  it('should return errors if both fail (Flag ON)', async () => {
    mockUseFeatureFlag.mockReturnValue(true);

    mockGetRootInvocationQuery.mockReturnValue({
      queryKey: ['rootInv'],
      queryFn: jest.fn().mockRejectedValue(new Error('Root error')),
      retry: false,
    });

    mockGetInvocationQuery.mockReturnValue({
      queryKey: ['legacyInv'],
      queryFn: jest.fn().mockRejectedValue(new Error('Legacy error')),
      retry: false,
    });

    const { result } = renderHook(() => useInvocationQuery('build-1234'), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.invocation).toBeUndefined();
    expect(result.current.errors).toHaveLength(2);
  });
});
