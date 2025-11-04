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

import { renderHook } from '@testing-library/react';
import { useState } from 'react';
import { act } from 'react';

import { FlatTreeItem, Graph } from './build_tree';
import { useKeyboardNavigation } from './use_keyboard_navigation';

const graph: Graph = {
  nodes: {
    root: {
      id: 'root',
      type: 'STAGE',
      label: 'root',
      status: 'FINAL',
      children: new Set(['child1', 'child2']),
      parents: new Set(),
    },
    child1: {
      id: 'child1',
      type: 'CHECK',
      label: 'child1',
      status: 'FINAL',
      children: new Set(['grandchild']),
      parents: new Set(['root']),
    },
    child2: {
      id: 'child2',
      type: 'CHECK',
      label: 'child2',
      status: 'UNKNOWN',
      children: new Set(),
      parents: new Set(['root']),
    },
    grandchild: {
      id: 'grandchild',
      type: 'CHECK',
      label: 'grandchild',
      status: 'FINAL',
      children: new Set(),
      parents: new Set(['child1']),
    },
  },
  roots: ['root'],
};

const visibleItems: FlatTreeItem[] = [
  {
    id: 'root',
    key: 'root-0',
    depth: 0,
    isRepeated: false,
    isCycle: false,
    hasChildren: true,
    parentId: null,
    isMatch: false,
  },
  {
    id: 'child1',
    key: 'root-child1-1',
    depth: 1,
    isRepeated: false,
    isCycle: false,
    hasChildren: true,
    parentId: 'root',
    isMatch: false,
  },
  {
    id: 'grandchild',
    key: 'child1-grandchild-2',
    depth: 2,
    isRepeated: false,
    isCycle: false,
    hasChildren: false,
    parentId: 'child1',
    isMatch: false,
  },
  {
    id: 'child2',
    key: 'root-child2-3',
    depth: 1,
    isRepeated: false,
    isCycle: false,
    hasChildren: false,
    parentId: 'root',
    isMatch: false,
  },
];

describe('useKeyboardNavigation', () => {
  describe('expansion', () => {
    it('should toggle recursive expansion', () => {
      const { result } = renderHook(() => {
        const [selectedKey, setSelectedKey] = useState<string | null>('root-0');
        const [expandedIds, setExpandedIds] = useState(new Set<string>([]));
        const { handleKeyDown } = useKeyboardNavigation(
          visibleItems,
          selectedKey,
          setSelectedKey,
          expandedIds,
          setExpandedIds,
          graph,
        );
        return { selectedKey, expandedIds, handleKeyDown };
      });

      act(() => {
        result.current.handleKeyDown({
          key: 'O',
          shiftKey: true,
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.expandedIds).toEqual(
        new Set<string>(['root', 'child1', 'grandchild', 'child2']),
      );

      act(() => {
        result.current.handleKeyDown({
          key: 'O',
          shiftKey: true,
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.expandedIds).toEqual(new Set<string>([]));
    });

    it('should toggle node expansion with Enter and space', () => {
      const { result } = renderHook(() => {
        const [selectedKey, setSelectedKey] = useState<string | null>('root-0');
        const [expandedIds, setExpandedIds] = useState(new Set<string>([]));
        const { handleKeyDown } = useKeyboardNavigation(
          visibleItems,
          selectedKey,
          setSelectedKey,
          expandedIds,
          setExpandedIds,
          graph,
        );
        return { selectedKey, expandedIds, handleKeyDown };
      });

      act(() => {
        result.current.handleKeyDown({
          key: 'Enter',
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.expandedIds).toEqual(new Set<string>(['root']));

      act(() => {
        result.current.handleKeyDown({
          key: ' ',
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.expandedIds).toEqual(new Set<string>([]));
    });

    it('should toggle node expansion', () => {
      const { result } = renderHook(() => {
        const [selectedKey, setSelectedKey] = useState<string | null>('root-0');
        const [expandedIds, setExpandedIds] = useState(new Set<string>([]));
        const { handleKeyDown } = useKeyboardNavigation(
          visibleItems,
          selectedKey,
          setSelectedKey,
          expandedIds,
          setExpandedIds,
          graph,
        );
        return { selectedKey, expandedIds, handleKeyDown };
      });

      act(() => {
        result.current.handleKeyDown({
          key: 'o',
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.expandedIds).toEqual(new Set<string>(['root']));

      act(() => {
        result.current.handleKeyDown({
          key: 'o',
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.expandedIds).toEqual(new Set<string>([]));
    });
  });

  describe('navigation', () => {
    it('should handle Home and End navigation', () => {
      const { result } = renderHook(() => {
        const [selectedKey, setSelectedKey] = useState<string | null>(
          'root-child1-1',
        );
        const [expandedIds, setExpandedIds] = useState(
          new Set<string>(['root', 'child1']),
        );
        const { handleKeyDown } = useKeyboardNavigation(
          visibleItems,
          selectedKey,
          setSelectedKey,
          expandedIds,
          setExpandedIds,
          graph,
        );
        return { selectedKey, expandedIds, handleKeyDown };
      });

      act(() => {
        result.current.handleKeyDown({
          key: 'End',
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.selectedKey).toBe('root-child2-3');

      act(() => {
        result.current.handleKeyDown({
          key: 'Home',
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.selectedKey).toBe('root-0');
    });

    it('should handle recursive collapse and expand', () => {
      const { result } = renderHook(() => {
        const [selectedKey, setSelectedKey] = useState<string | null>('root-0');
        const [expandedIds, setExpandedIds] = useState(
          new Set<string>(['root', 'child1']),
        );
        const { handleKeyDown } = useKeyboardNavigation(
          visibleItems,
          selectedKey,
          setSelectedKey,
          expandedIds,
          setExpandedIds,
          graph,
        );
        return { selectedKey, expandedIds, handleKeyDown };
      });

      act(() => {
        result.current.handleKeyDown({
          key: 'L',
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.expandedIds).toEqual(
        new Set<string>(['root', 'child1', 'grandchild', 'child2']),
      );

      act(() => {
        result.current.handleKeyDown({
          key: 'H',
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.expandedIds).toEqual(new Set<string>([]));
    });

    it('should handle collapse and expand', () => {
      const { result } = renderHook(() => {
        const [selectedKey, setSelectedKey] = useState<string | null>(
          'child1-grandchild-2',
        );
        const [expandedIds, setExpandedIds] = useState(
          new Set<string>(['root', 'child1']),
        );
        const { handleKeyDown } = useKeyboardNavigation(
          visibleItems,
          selectedKey,
          setSelectedKey,
          expandedIds,
          setExpandedIds,
          graph,
        );
        return { selectedKey, expandedIds, handleKeyDown };
      });

      act(() => {
        result.current.handleKeyDown({
          key: 'h',
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.selectedKey).toBe('root-child1-1');

      act(() => {
        result.current.handleKeyDown({
          key: 'l',
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.selectedKey).toBe('child1-grandchild-2');
    });

    it('should handle sibling navigation', () => {
      const { result } = renderHook(() => {
        const [selectedKey, setSelectedKey] = useState<string | null>(
          'root-child1-1',
        );
        const [expandedIds, setExpandedIds] = useState(
          new Set<string>(['root', 'child1']),
        );
        const { handleKeyDown } = useKeyboardNavigation(
          visibleItems,
          selectedKey,
          setSelectedKey,
          expandedIds,
          setExpandedIds,
          graph,
        );
        return { selectedKey, expandedIds, handleKeyDown };
      });

      act(() => {
        result.current.handleKeyDown({
          key: 'J',
          shiftKey: true,
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.selectedKey).toBe('root-child2-3');

      act(() => {
        result.current.handleKeyDown({
          key: 'K',
          shiftKey: true,
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.selectedKey).toBe('root-child1-1');
    });

    it('should handle basic navigation', () => {
      const { result } = renderHook(() => {
        const [selectedKey, setSelectedKey] = useState<string | null>('root-0');
        const [expandedIds, setExpandedIds] = useState(
          new Set<string>(['root', 'child1']),
        );
        const { handleKeyDown } = useKeyboardNavigation(
          visibleItems,
          selectedKey,
          setSelectedKey,
          expandedIds,
          setExpandedIds,
          graph,
        );
        return { selectedKey, expandedIds, handleKeyDown };
      });

      act(() => {
        result.current.handleKeyDown({
          key: 'j',
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.selectedKey).toBe('root-child1-1');

      act(() => {
        result.current.handleKeyDown({
          key: 'k',
          preventDefault: () => {},
        } as React.KeyboardEvent);
      });
      expect(result.current.selectedKey).toBe('root-0');
    });
  });
});
