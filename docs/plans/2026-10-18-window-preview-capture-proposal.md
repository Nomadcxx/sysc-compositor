# Window preview capture contract proposal

Kind: gate check. Scope: sysc-shell `sysc-787` / GitHub #299.

## Proposal

Use the standard Wayland window-capture path:

1. `ext_foreign_toplevel_list_v1` exposes one handle for each mapped window.
   Its identifier is the decimal Niri window ID, so sysc-shell can join the
   handle to the stable ID it already uses for switcher actions.
2. `ext_foreign_toplevel_image_capture_source_manager_v1` creates a source for
   that handle.
3. `ext_image_copy_capture_manager_v1` creates the session and frames used to
   copy the source into sysc-shell.

The source represents one mapped window's scene subtree. It must not include
other windows or the output backdrop. The session's `buffer_size` describes the
source in buffer pixels and follows the mapped window's current geometry. The
shell associates every frame with the same stable window ID and current size
generation; it discards frames from a previous size after a resize.

## Frame and damage behavior

- Capture an initial full frame when the switcher first requests the preview.
- Keep at most one frame outstanding per session. The protocol also forbids a
  second frame object before the first is destroyed.
- After each ready frame, the client destroys it and arms the next frame. The
  compositor holds that request until the window scene is damaged, then
  coalesces the changed region and reports it through the frame's `damage`
  events. Do not run a periodic capture timer or copy unchanged pixels.
- On resize, send updated buffer constraints before capturing at the new size.
  The shell drops any frame whose size generation is stale and reallocates from
  the latest constraints.
- Treat `buffer_size` as untrusted input: reject zero or overflowing dimensions
  and cap each allocation at the existing 64 MiB capture limit.
- A frame failure or unsupported capture removes only the preview. The
  switcher keeps the window's existing identity, title, and actions.

## Ownership and teardown

Keep native capture objects with the mapped-window owner in `internal/wlr`,
alongside the toplevel's scene tree and lifecycle listeners. Give each mapped
window a capture-only scene tree containing its client surface tree; do not
capture the output scene and crop it. Mango 0.18.0 creates a separate XDG or
Xwayland surface tree per client, lazily creates the source only after its
policy check, and destroys the capture tree on unmap. Reuse that ownership and
lifecycle pattern, and track each session until its destroy event removes the
listener and session state.

When the switcher closes, sysc-shell destroys its outstanding frame, session,
source, then foreign-toplevel handle. When a window unmaps or closes, the
compositor stops its sessions and releases their listeners and source before
destroying the window scene and closing the handle. A session stopped by the
compositor cannot produce another frame.

## Privacy

Treat this path as `ScreenCapture`, following Niri's `RenderTarget::ScreenCapture`
semantics. Before accepting a source request, refuse capture for any window
whose rule blocks `screen-capture`. If a rule changes while capture is active,
stop that session before another frame can be delivered. Keep that window in the
switcher list without a preview.

A `screencast`-only rule remains scoped to screencast rendering and does not
block this preview path. Mango's `shield_when_capture` is a useful example of
checking policy before source acceptance, but its policy is not adopted here.

## Format negotiation

Start with SHM, which matches sysc-shell's existing capture buffer path. Use the
session's advertised `buffer_size` and SHM formats as the allocation contract.
Advertise only formats the renderer can read back or convert to and sysc-shell
can consume. If renderer capabilities change, update the constraints before
accepting another buffer. sysc-shell allocates from the latest constraints and
treats unsupported formats or allocation failure as a missing preview, not a
switcher failure. Add DMA-BUF only if a measured requirement justifies it.

Mango's 0.18.0 capture code checks renderer support before adding SHM formats.
Keep that check; do not copy its unconditional format assumptions or its
`shield_when_capture` rule.

## Go and cgo boundary

Implement policy, window/source/session ownership, damage coalescing, and
cleanup in Go on the compositor thread. Use wlroots 0.20 through cgo for the
Wayland objects and renderer operations. Keep any listener trampoline in a Go
file's cgo preamble. Do not add standalone `.c` or `.h` files or introduce
another implementation language.

Mango is a wlroots API reference, not code to port. Its capture source, session
tracking, and format-negotiation patterns inform this contract; its C
implementation and product-specific capture policy do not.

## Boundary

This is a capture-contract gate, not an M11 or M12 implementation plan. M11's
`ScreenshotWindow` remains one-shot and does not provide a live stream. The
compositor owner must schedule source support in the milestone that owns window
scene and capture work. No preview implementation is part of this proposal.

References: Niri `v26.04` (`8ed0da44d974c32c6877d2f4630c314da0717ecb`),
mangowm `0.18.0` (`a21b10fcbe6156b76ea96178d1941de5cb36137f`), and the staging
protocols `ext-foreign-toplevel-list-v1`, `ext-image-capture-source-v1`, and
`ext-image-copy-capture-v1`.
