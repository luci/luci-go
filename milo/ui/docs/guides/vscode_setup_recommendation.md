# Extensions

Here's a list of recommended extensions to make your development experience
smoother.

## Jest

This let you run/debug Jest unit tests.

You may want to set the run mode to "on-demand". Otherwise automated test
execution may slow your IDE down because the transpilation step can take some
time.

```json
// In VSCode `settings.json`.
{
  // Prevents automatic test execution on save.
  "jest.runMode": "on-demand",
}
```

Also, when there are more than one Jest enabled project in your VSCode workspace
(e.g. when you open vscode in `./luci` directory instead of `./luci/milo/ui`
directory), you may not be able to run jest unit tests.
To fix this, you can [setup a multi-root VSCode workspace](#set-up-a-multi-root-vscode-workspace).

## ESLint

This let you see whether your code satisfy the linter as you edit the code.

If `luci/milo/ui` is not your VSCode workspace root, ESLint may fail to resolve
some import statements.

To resolve this issue, you can either

1. [setup a multi-root VSCode workspace](#set-up-a-multi-root-vscode-workspace).
   and add `luci/milo/ui` as one of the root, or
2. add `luci/milo/ui` to `"eslint.workingDirectories"` in VSCode
   `settings.json`.

```json
// In VSCode `settings.json`.
{
	"eslint.workingDirectories": [
		// Allow ESLint to resolve the dependencies correctly when there are
		// multiple ESLint enabled projects in the VSCode workspace.
		{
			"mode": "auto"
		}
	],
}
```

You can also configure the extension to apply the fix automatically on save.
```json
// In VSCode `settings.json`.
{
	// Applies ESLint fixes automatically on save.
	"editor.codeActionsOnSave": {
			"source.fixAll.eslint": "explicit"
	},
}
```

# Other configurations

## Set up a multi-root VSCode workspace

Create a `<workspace-name>.code-workspace` file. You can then open the workspace
with VSCode.

The following example is created in `<luci_root>/.vscode/luci.code-workspace`.
`<luci_root>/.vscode` is Git ignored, suitable for storing your customized
VSCode config. If you create it in another place, you will need to update the
path accordingly.

```json
// In `<luci_root>/.vscode/luci.code-workspace`.
{
	"folders": [
		{
			"path": ".."
		},
		{
			"path": "../milo/ui"
		},
		{
			"path": "../path/to/other/projects/you/work/on"
		},
	]
}
```
