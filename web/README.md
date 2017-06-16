# web/

Common html/css/js files shared by all apps live here, as well as a scripts
to "build" web applications.

## Setup

*Hint*: If you are using infra.git gclient solution, you can skip the following
steps after running `eval go/env.go`.

1.  Install node & npm: https://docs.npmjs.com/getting-started/installing-node
1.  Install the rest: `./web.py install`

## Building apps

All supported web apps are in `web/apps/`. The snippet below uses rpcexplorer
as an example.

```shell
cd web
./web.py gulp rpcexplorer clean  # this is optional
./web.py build rpcexplorer
ls -R dist/rpcexplorer           # the built app is here
```

`dist/rpcexplorer` then can be deployed as is to a server.

## Layout

### Applications (apps/)

Web applications are defined as subdirectories in the `apps/` directory. Each
subdirectory contains an application root which, in turn, contains the composite
application files.

The layout of the `apps/` directory is hard-coded into the Gulp build script
located at `apps/gulp-common.js`.

The application layout looks like:

`apps/myapp/...`

* `gulpfile.js` is the application's Gulpfile. It performs minimal application-
  specific tailoring of the common Gulp environment and then calls through to
  `apps/gulp-common.js`.
* `inc` is a symlink to `web/inc` directory so that common code can be
  referenced within the app root.
* `index.html` is the application's main entry point.
* `elements/elements.html` contains the set of import links for vulcanization.
* `scripts-ts/*.ts` is the set of application TypeScript code, and will be
  automatically trans-compiled to JavaScript if defined.

### Includes (inc/)

The "inc/" directory contains shared web resources:

- Local resources are LUCI-specific web resources that belong to this
  repository. They are located in immediate subdirectories of `inc`.
- Bower-imported resources are defined in `bower.json`, and are located in the
  `inc/bower_components` directory.

Web apps that want to use these shared resources can do so by symlinking the
`inc` directory into their application subdirectory (`web/apps/<app>/inc`).

#### TypeScript

The `inc/tsconfig.json` file in this directory is the common TypeScript
configuration file for the TypeScript files in this directory.

TypeScript is compiled by the 'ts' build target in `apps/gulp-common.js`. The
compiled JavaScript is merged into a single output file, `main.js`, along with
an associated map file, `main.js.map`. These generated files are not checked in,
and are ignored via ".gitignore".

Local LUCI TypeScript in the `inc/` directory is composed of a series of
internal relative references. Each referenced TypeScript file will export its
contents as `namespace`s. We do this to avoid unnecessary module imports:

```TypeScript
////////////////////////////////////////////////////////////////////////////////
// inc/component/importer.ts
////////////////////////////////////////////////////////////////////////////////

///<reference path="../relpath/file.ts" />

MyThing.doSomething();

////////////////////////////////////////////////////////////////////////////////
// inc/relpath/file.ts
////////////////////////////////////////////////////////////////////////////////
namespace MyThing {
  function doSomething() { ... }
}
```

An application that wants to use this TypeScript can do so by creating a
`scripts-ts` subdirectory in its application folder. Any TypeScript in this
subdirectory will be automatically compiled into `main.js` at build time and can
then be included in the application.

Minimally, an application should reference the `inc/` packages that it cares
about to pull them in. This can be done with a simple reference (remember that
`inc` is symlinked from the application's root):

```TypeScript
////////////////////////////////////////////////////////////////////////////////
// apps/myapp/scripts-ts/main.ts
////////////////////////////////////////////////////////////////////////////////

///<reference path="../inc/component/importer.ts" />
```

This will cause Gulp to build a `main.js` file containing the correctly-ordered
chain of referenced components ready for inclusion.

If an application chooses to import external modules, those imports will require
a module loader such as [require.js](http://requirejs.org/) to be installed in
the application.
