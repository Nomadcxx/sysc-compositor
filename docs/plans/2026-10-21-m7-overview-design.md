# M7 Overview and Input Design

**Date:** 2026-10-21

**Status:** Approved

## Goal

Implement the M7 behavior in the compositor: overview, three- and four-finger swipe gestures, hot corners, tiled-window drag insertion hints, and drag-and-drop edge scrolling. Keep the Niri 26.04 KDL, IPC, CLI, and input contract. sysc-shell remains a Wayland client and does not draw the overview.

## Rendering ownership

The WM model owns overview state and the geometry that lays out each output's workspaces as a vertical strip. The active workspace sits at the output center. `overview.zoom` scales each workspace, and `overview.backdrop-color` fills the space behind them.

`internal/wlr` owns the compositor projection scene. It reuses references to the buffers already held by each live xdg scene tree, then applies the workspace position and per-buffer destination size needed for the overview projection. The existing xdg trees remain responsible for surface commits, subsurfaces, popups, output enter/leave, and client lifecycle. The projection refreshes when source surfaces commit and removes its nodes when their source windows unmap or are destroyed. It does not ask clients to resize for the overview.

Pointer hit-testing uses the inverse workspace transform and the WM's window placements. Clicking a window activates that window and its workspace, then closes the overview. Clicking empty space inside a workspace selects that workspace and closes the overview. IPC reports overview state through the existing Niri `OverviewState` request and `OverviewOpenedOrClosed` event shapes.

## Input behavior

- Three-finger swipes choose an axis after 16 logical pixels of accumulated movement. Vertical movement switches workspaces. Horizontal movement scrolls the column view for the workspace under the pointer while overview is open, or the active workspace otherwise.
- Four-finger vertical swipes track overview progress. Releasing settles to open or closed based on the projected gesture position. M8 owns spring animations; M7 uses direct state changes outside the gesture.
- A configured hot corner occupies the one-logical-pixel corner region on each output. Entering it toggles overview once. `off` disables every corner. If no corner is configured, the top-left corner is enabled by default, matching Niri.
- A window move starts after the pointer crosses Niri's drag threshold. The WM tracks the source tile and the pointer's target. Tiled moves show an insert hint and commit the target position on release; cancellation restores the source. Floating moves retain their direct position behavior.
- The existing wlroots data-device manager supplies client drag-and-drop events. While a drag remains near a workspace's horizontal edge, the layout scrolls after the configured delay and scales to the configured maximum speed.

## Configuration

M7 decodes the Niri fields needed by these behaviors: `overview.zoom`, `overview.backdrop-color`, `gestures.hot-corners`, `gestures.dnd-edge-view-scroll`, and `layout.insert-hint.off` / `color`. Defaults match Niri 26.04. In particular, edge view scrolling defaults to a 30 logical pixel trigger width, a 100 ms delay, and a maximum speed of 1500 logical pixels per second.

## Verification

Pure Go tests cover workspace projection geometry, inverse hit-testing, three-finger axis selection, gesture completion, hot-corner entry, drag target selection, cancellation, and edge-scroll timing. Nested wlroots tests cover live buffer updates, projection cleanup, client surface lifecycle, pointer selection, and compositor-thread input handling. The M7 laptop gate uses the machine's existing Niri config and exercises overview actions, gestures where available, tiled dragging, and sysc-shell IPC compatibility.

The overview renderer must pass a focused nested viability check for buffer sharing, surface commits, and subsurface teardown before the rest of M7 depends on it. If wlroots buffer ownership or output tracking does not support the design, stop and revise the renderer before adding more behavior.
