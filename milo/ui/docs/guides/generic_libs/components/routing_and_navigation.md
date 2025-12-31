# Routing & navigation components

Components that handle routing, generic tabs, and navigation logic.

## RoutedTabs

`RoutedTabs` is a wrapper around MUI's `Tabs` component that integrates
seamlessly with `react-router`. It allows tabs to be statefully managed via the
URL and renders content using nested routes.

### Usage

```tsx
import { RouterProvider, createBrowserRouter, Route } from 'react-router';
import { RoutedTabs, RoutedTab } from '@/generic_libs/components/routed_tabs';
import { useDeclareTabId } from '@/generic_libs/components/routed_tabs';

// 1. Define your tab components.
// Each component MUST declare its associated tab ID using `useDeclareTabId`.
function TabAContent() {
  useDeclareTabId('tab-a');
  return <div>Content A</div>;
}

function TabBContent() {
  useDeclareTabId('tab-b');
  return <div>Content B</div>;
}

// 2. Define your routes layout.
// The parent route renders <RoutedTabs>.
function MyPage() {
  return (
    <div>
      <h1>My Page</h1>
      <RoutedTabs>
        <RoutedTab value="tab-a" to="a" label="Tab A" />
        <RoutedTab value="tab-b" to="b" label="Tab B" />
      </RoutedTabs>
      {/*
         Note: <RoutedTabs> includes an <Outlet /> internally.
         However, you usually don't need to manually place <Outlet />
         unless you want to control where the content renders relative to tabs
         (but RoutedTabs structure is fixed to Render tabs then outlet).

         Actually, RoutedTabs implementation renders:
         <ActiveTabContextProvider ...>
           <Tabs ... />
           <Outlet />
         </ActiveTabContextProvider>

         So the content will appear immediately below the tabs.
      */}
    </div>
  );
}

// 3. Configure the router
const router = createBrowserRouter([
  {
    path: '/my-page',
    element: <MyPage />,
    children: [
      { path: 'a', element: <TabAContent /> },
      { path: 'b', element: <TabBContent /> },
      // Redirect to default tab
      { index: true, element: <Navigate to="a" replace /> },
    ],
  },
]);
```

### Internal workings

`RoutedTabs` uses a combination of React Context and `useReducer` to manage the
active tab state.

1.  **State Management**: It maintains the `activeTab` state internally using
    `useReducer`.
2.  **Context**: It provides two contexts:
    *   `ActiveTabContext`: Exposes the currently active `tabId`.
    *   `ActiveTabUpdaterContext`: Exposes a `dispatch` function to update the
        active tab.
3.  **Registration**: Child components (the localized route content) call
    `useDeclareTabId(id)`. This hook dispatches an action to register the tab ID
    as active when the component mounts.
4.  **Syncing**: `RoutedTabs` passes the `activeTab.tabId` to the MUI `Tabs`
    component as the `value`.

This inversion of control (content declaring its ID) ensures that the active tab
in the UI always matches the content actually being rendered by the router,
preventing sync issues.

### API

#### `RoutedTabs`
Inherits props from MUI `Tabs`, but omits `value`.

#### `RoutedTab`
Inherits props from MUI `Tab`, plus:
*   `value`: The unique ID of the tab (must match the ID declared in
    `useDeclareTabId`).
*   `to`: The route path to link to.
*   `hideWhenInactive`: (Optional) If true, the tab button is hidden when not
    active. Useful for conditional tabs that should only appear when reachable.
