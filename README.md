luci-go: Swarming and Isolate servers in Go
===========================================

Installing
----------

    go get -u github.com/luci/luci-go/client/cmd/isolate
    go get github.com/luci/luci-go/client/cmd/isolateserver
    go get github.com/luci/luci-go/client/cmd/swarming


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
    cd $GOROOT/github.com/luci/luci-go
    git checkout -b work origin/master
    # hack hack
    git commit -a -m "This is awesome\nR=joe@example.com"
    # This will ask for your Google Account credentials.
    git cl upload -s
    # Wait for LGTM over email.
    git cl land
    # Use `git cl help` and `git cl help <cmd>` for more details.
