# web/

Common html/css/js files shared by all apps live here, as well as a scripts
to "build" web applications.

## Setup

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
