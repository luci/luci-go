# Fleet Console Layouts

This directory contains the layout components specific to the Fleet Console application.

## Custom Components

Unlike other LUCI apps that use shared components from `src/common/layouts/app_bar`, Fleet Console uses its own custom header and menu components to provide a tailored experience:

- **Header** ([header.tsx](./header.tsx)): The top app bar for Fleet Console.
- **SettingsMenu** ([settings_menu.tsx](./settings_menu.tsx)): The dropdown menu accessible from the header (vertical dots). This is the Fleet Console equivalent of `AppMenu`.
- **Sidebar** ([sidebar.tsx](./sidebar.tsx)): The side navigation menu.

## Guidelines

When adding global links or actions (like "Version" info or promoter links), ensure they are added to both the shared components (for other apps) and these custom components (for Fleet Console) if they are intended to be visible everywhere.
