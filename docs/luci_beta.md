# LUCI Beta
Welcome to the LUCI beta for Chrome! LUCI is the Chrome Operations team's replacement for BuildBot.
LUCI runs 100% on the cloud and scales really well for large projects.

## Current Status
Opting into LUCI beta means these builder(s) will run entirely on the LUCI stack:
* linux_chromium_rel_ng

You'll know it's a LUCI builder because it will show a LUCI chip in Gerrit:

![LUCI chip in Gerrit](https://chromium.googlesource.com/infra/luci/luci-go/+/master/docs/images/luci_chip.png)

The link will take you to our new web UI that was made to look 
almost exactly like Buildbot to make the transition easy 
[example](https://luci-milo.appspot.com/swarming/task/3852c903738e9910).

## Signing up
Please email luci-eng@google.com if you would like to opt-in to Beta dogfood, 
and we will add you to [this group](https://chrome-infra-auth.appspot.com/auth/groups/luci-chromium-cq-dogfood)

## Bypass LUCI for one CL
You can have your CL switch back to using Buildbot builders 
by adding to your CL [description footer](https://chromium-review.googlesource.com/c/chromium/src/+/541299/4..5//COMMIT_MSG)
```
No-Equivalent-Builders: true
```

## Feedback
Enjoy LUCI! We're really interested in your feedback so please 
don't hesitate to send it whether good or bad via email or 
[on the bug tracker](https://bugs.chromium.org/p/chromium/issues/entry?labels=LUCI-ClosedBeta-Bug&components=Infra%3EPlatform&summary=%5BLUCI-Beta-Bug%5D%20Enter%20an%20one-line%20summary&cc=nodir@chromium.org,%20estaab@chromium.org,%20efoo@chromium.org&description=Please%20use%20this%20to%20template%20to%20describe%20the%20issue%20you%20are%20encountering%20with%20LUCI.%0A%0AInclude%20the%20following%20information%20in%20this%20bug%3A%0A-%20Problem%2FBug%0A-%20Relevant%20LUCI%20builder%0A-%20Links%20%28i.e.%20links%20to%20a%20failing%20CL%29%0A-%20Expected%20outcome%0A%0AIn%20addition%2C%20please%20clearly%20specify%20the%20priority%2Fseverity%20of%20your%20issue.%20). 

