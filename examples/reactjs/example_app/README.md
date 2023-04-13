# Example ReactJS app

## Introduction

This is an example ReactJS app that you can copy/paste in your project.
It uses multiple state-management solutions, you can opt out of any of them.
It also has all the configurations in place for you to start developing your app without the need of any wiring.

It follows the recommendations mentioned in [Luci test react recommendations](http://go/luci-test-tools-reactjs).

The gist of those recommendations are:
1. ReactJS as the base framework.
2. TypeScript for languange.
3. React Router for routing.
4. NPM for packaging.
5. MaterialUI for the UI components package.
6. React Query and Redux as two example of state-management.
7. Jest and React testing library for unit testing.
8. Cypress for end-to-end testing.
9. Eslint for linting with a small number of opinionated linting configurations (can be found in `eslintrc.js` in the rules section).

## Adding your apps meta data

Before running and building the app, you need to fill in some data about it:

1. In `package.json` replace `app_name` in `name` with your apps name.
2. Replace `{USER}` with the username of the app's author.
3. In `cypress.json` replace `{APP_BASE_URL}` with your app's base URL.

## Deleting framework folders you don't need

Delete the items corresponding the framework you won't use.

1. **Redux**:
   1. `store` folder.
   2. `react-redux` and `@redux/toolkit` from `package.json`.
2. **React Query**:
   1. `example_react_query` folder.
   2. `react-query` from `package.json`.


## Getting ESLint to work with TypeScript paths in the VSCode

To get `TypeScript`'s paths (such as `@/`) working with ESLint
you will need to add the following line to your vscode settings:

```json
"eslint.workingDirectories": [
   {
      "mode": "auto"
   }
],
```

This is because we use a monorepo and we need to localize the `tsconfig`
loaded for vscode's ESlint plugin.

## Building and running the project
After making the modifications mentioned above and you have a working project,
you can use the following commands:

1. Run with watch for development: `npm run watch`.
2. Run tests: `npm run test`.
3. Run End to End tests with Cypress: `npm run e2e`.
4. Fix eslint-fixable issues: `npm run fix-eslint`.
5. Type check: `npm run typecheck`.
6. Build the project for production: `npm run build`.