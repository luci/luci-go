client-go: Swarming and Isolate servers clients in Go
=====================================================

See https://code.google.com/p/swarming/ for more details.


Installing
----------

    go get -u chromium.googlesource.com/infra/swarming/client-go/cmd/isolate
    go get chromium.googlesource.com/infra/swarming/client-go/cmd/isolateserver
    go get chromium.googlesource.com/infra/swarming/client-go/cmd/swarming


Code layout
-----------

  * `/cmd/...` contains executables.
  * `/internal/...` contains non API shared internal packages for use by other
    packages in this repository. This includes third parties. See
    https://golang.org/s/go14internal for more details.
  * `/...` not in any other of the category is an API package usable externally.


Versioning
----------

  * Branch `go1` contains the stable code.
  * Branch `master` constains the latest code.


Contributing
------------

  * Sign the [Google CLA](https://cla.developers.google.com/clas).
  * Make sure your `user.email` and `user.name` are configured in `git config`.

    git clone https://chromium.googlesource.com/chromium/tools/depot_tools.git $HOME/src/depot_tools
    PATH="$PATH:`$HOME/src/depot_tools"
    cd $GOROOT/chromium.googlesource.com/infra/swarming/client-go
    git checkout -b work origin/master
    # hack hack
    git commit -a -m "This is awesome\nR=joe@example.com"
    # This will ask for your Google Account credentials.
    git cl upload -s
    # Wait for LGTM over email.
    git cl land
    # Use `git cl help` and `git cl help <cmd>` for more details.
