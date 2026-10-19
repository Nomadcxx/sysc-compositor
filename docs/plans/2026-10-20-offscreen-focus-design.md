# Offscreen Window Focus Output

## Problem

M4 has mapped toplevels but no independent window-to-output assignment. The compositor infers an
output from each window center. When a mapped window's center lies outside every output, `focusView`
retains the active output while `updateKeyboardFocus` rejects that window. `FocusWindow` then reports
success without moving keyboard focus. The Niri snapshot already assigns such a window to the
focused output.

## Decision

Add one compositor helper that resolves a view's output from its center and falls back to the current
connected focused output when the center lies outside every output. Use that result in `focusView`,
`updateKeyboardFocus`, `Windows`, and `FocusWorkspaceOutput`.

This gives focus routing, workspace selection, and the Niri snapshot the same M4 output assignment.
The helper leaves window geometry unchanged. Existing layer keyboard-focus selection and
on-output behavior remain intact. The Niri server keeps its current fallback for missing or
disconnected output names.

M5 will add layout-owned placement. This change keeps the current M4 inference in one place and does
not introduce layout policy, configuration, or IPC fields.

## Alternatives considered

1. **Resolve the output once for each view.** This keeps the compositor's focus and snapshot paths
   consistent and leaves window placement alone. Use this approach.
2. **Allow an offscreen retained view only in `updateKeyboardFocus`.** This is a smaller change, but
   `WindowInfo.Output` remains empty while the server applies its own fallback.
3. **Move the view onto the active output before focusing it.** This changes window geometry as a
   side effect and adds placement behavior before M5.

## Verification

Add a nested wlroots regression with two mapped `weston-terminal` clients. Place one view far outside
the output, call `FocusWindow`, and check actual keyboard focus and the reported output. Keep the
existing on-output and layer-focus checks. Run the server's focused offscreen-output fallback test.
Use the laptop testbench for a nested runtime check when reachable; do not install the binary or
change greetd.
