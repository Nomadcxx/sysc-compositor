# Local godbus backport

Source: github.com/godbus/dbus/v5 v5.2.2, under its original LICENSE.

The portal descriptor regression exposed a decoder mismatch: container types used UnixFDIndex
while attached descriptors decoded as UnixFD. The original panic recovery could report success
with an empty body and leak descriptors before the portal handler saw them.

The local changes match container types to decoded values, preserve decode errors, reject
invalid descriptor indexes/counts and duplicate dictionary keys, and close unused/failed
descriptor transfers. `fd_decode_regression_test.go` exercises the native Unix transport.

From the compositor repository:

```
timeout 90s env GOMAXPROCS=2 go test -count=1 github.com/godbus/dbus/v5 -run '^TestReceivedDescriptorContainersAndCleanup$'
```

Keep this replacement until an upstream release passes that test and the portal descriptor
regression. The reproducible patch lives in docs/plans/assets/2026-10-28-godbus-fd-fix.patch.
