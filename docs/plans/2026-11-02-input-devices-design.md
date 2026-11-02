# Input devices and native sysc-shell support (tranche 2)

Owner direction 2026-11-02: touchpad control is implemented in sysc-compositor and in
sysc-shell, and sysc-shell gains native sysc-compositor support. That support should be easy,
because the shell already targets Niri's IPC, which sysc-compositor speaks.

This is tranche 2 of three. It comes after [tranche 1, identity](2026-11-02-own-identity-design.md),
which moves the config directory and gives sysc-shell its compositor-aware config directory and
IPC spawning.

**Plans.**
- Compositor: [2026-11-02-input-devices.md](2026-11-02-input-devices.md).
- Shell: `/home/nomadx/sysc-shell/docs/plans/2026-11-02-sysc-compositor-native-support.md`, gitignored
  by that repository's rule. Its phase A belongs to tranche 1, its phase B to this tranche.

## Why

- **Settings are ignored.** sysc-compositor applies no libinput settings, so every `input { touchpad
  { … } }` setting in a user's config is silently ignored. That includes Niri's default
  `tap` and `natural-scroll`. Tap to click, natural scrolling, pointer speed and
  disable-while-typing all stay at libinput's defaults.
- **Gestures disagree.** The 3-finger swipe's two axes assume opposite natural-scroll settings,
  found in the
  [M15 addendum](2026-10-31-m15-navigation-binds.md#addendum-2026-11-02-overview-input-follow-up).
- **No settings page.** sysc-shell has none for any input device. Users edit KDL by hand.

## Goals

1. **Compositor support.** sysc-compositor parses and applies Niri's pointing-device
   sections. Changes apply when a device appears and on every config reload, with no restart.
   The sections are:
   - `input { touchpad, mouse, trackpoint, trackball }`, with every field Niri 26.04 accepts;
   - `scroll-factor`, which the compositor applies itself.
2. **Gestures follow natural scrolling.** The 3-finger swipe and the overview scroll gesture follow
   the touchpad's natural-scroll setting, as in Niri.
3. **A shell settings page.** It has touchpad and mouse controls and works on both compositors:
   under sysc-compositor and under Niri it writes the same KDL.
4. **Native sysc-compositor support in sysc-shell.**
   - Recognise the running compositor.
   - Package without a hard dependency on Niri.
   - Describe itself as a shell for both.

   Config directory and spawning arrive in tranche 1.

## Non-goals

- **Tablets, touchscreens and keyboards.** Tablet settings are M12. Keyboard xkb exists. Touch
  stays as it is.
- **A runtime IPC request to change input settings.** Niri has none, and the config file is
  the one route that works on both compositors and persists.
- **Per-device overrides by name.** Niri has none either.
- **Gesture continuity.** The discrete vertical swipe stays as it is (a following, continuous
  workspace switch is separate work).

## Anti-goals

- **Silently dropping settings.** A shell setting must never discard a setting it doesn't show
  (see D5).
- **Changing scroll direction without being asked.** A user who never set `natural-scroll` keeps
  libinput's default.
- **New C beyond libinput's configuration calls.** The owner approved libinput through cgo on
  2026-11-02. Anything else, such as libudev, needs a new approval.

## Decisions

### D1. Config: Niri's pointing-device sections

- **Fields.** `internal/config` decodes `touchpad`, `mouse`, `trackpoint` and `trackball` with
  Niri 26.04's fields and value ranges (`niri-config/src/input.rs`):
  - for all four: `off`, `natural-scroll`, `accel-speed` (−1…1), `accel-profile`
    (adaptive|flat), `scroll-method` (no-scroll|two-finger|edge|on-button-down),
    `scroll-button`, `scroll-button-lock`, `left-handed` and `middle-emulation`;
  - for the touchpad also: `tap`, `dwt`, `dwtp`, `drag`, `drag-lock`, `click-method`
    (button-areas|clickfinger), `tap-button-map` (left-right-middle|left-middle-right) and
    `disabled-on-external-mouse`;
  - `scroll-factor` (a number, or `vertical`/`horizontal` values) for the touchpad and the mouse.
- **Include merging.** A device section from a later include *replaces* an earlier one; it is not
  merged field by field. This is Niri's documented rule for pointing-device sections. A test
  pins it.
- **Validation.** Bad values are validate errors with Niri's messages. These sections leave the
  "unapplied" warning list.

### D2. Applying settings through libinput

- **Access.** A new cgo file in `internal/wlr` includes `<libinput.h>` and `<wlr/backend/libinput.h>`
  (`pkg-config: libinput`). For each pointer device where `wlr_input_device_is_libinput` is true,
  it gets the handle with `wlr_libinput_get_device_handle` and makes the
  `libinput_device_config_*` calls. Nothing else in libinput is used.
- **When.** Settings are applied when a pointer device is added, and to every device after a
  config reload that changes `input`.
- **Device kind, in Niri's order** (`apply_libinput_settings`):
  - A touchpad has a tap finger count above 0.
  - A trackpoint carries udev `ID_INPUT_POINTINGSTICK`.
  - A trackball carries `ID_INPUT_TRACKBALL`.
  - Everything else is a mouse.
- **Reading udev without libudev.** The udev properties are read in pure Go from
  `/run/udev/data/c<major>:<minor>`. The major and minor come from
  `libinput_device_get_sysname`, through the device's `dev` file in sysfs.
- **Unsupported devices.** A setting the device doesn't support is skipped, as Niri does.
  libinput reports this as `LIBINPUT_CONFIG_STATUS_UNSUPPORTED`.
- **Nested backends** have no libinput devices, so nothing is applied. A unit test covers how
  settings map to libinput calls through a seam. The real calls are proved in the session VM and
  on the laptop.

### D3. scroll-factor

`handleCursorAxis` multiplies the delta and the v120 value by the device kind's factor before
binds, the overview gesture and clients see them, as Niri's `on_pointer_axis` does. The factor
is per kind (touchpad or mouse) and per axis.

### D4. Gestures follow natural scrolling

- **The rule.** The 3-finger swipe takes Niri's rule:
  - if the touchpad's natural scrolling is on, both swipe deltas are negated;
  - then a positive vertical delta means down, and a positive horizontal delta moves the view
    right (`on_gesture_swipe_update`).
- **Overview scroll gesture.** It currently flips the vertical axis to match today's swipe. That
  flip goes, because both now share Niri's convention.
- **Natural-scroll state.** Read from the libinput device (`config_scroll_get_natural_scroll_enabled`),
  so it matches what libinput applies.
- **Correction to the M15 record.** It said the overview gesture's signs follow Niri; they keep
  doing so, but now through the corrected swipe. The addendum's "found, not fixed" item is
  closed by this tranche.

### D5. sysc-shell: input settings page

**The page.** A new Settings page, "Mouse & touchpad", following the existing Settings registry
and page patterns:
- **Touchpad:** tap to click, natural scrolling, pointer speed (`accel-speed`), acceleration
  profile, disable while typing (`dwt`), scroll method (two-finger or edge), click method, scroll
  speed (`scroll-factor`), and off while an external mouse is connected.
- **Mouse:** natural scrolling, pointer speed, acceleration profile, left-handed, and scroll
  speed.

The page's touchpad half is hidden when no input device carries udev's
`ID_INPUT_TOUCHPAD=1`. The shell reads that from `/run/udev/data/`, in pure Go (my call).

**How settings are written.** The shell writes them to a managed include, `sysc-shell-input.kdl`,
in the running compositor's config directory: the one tranche 1 gave sysc-shell's theming. It
adds `include optional=true "sysc-shell-input.kdl"` as the **last** top-level line of the
config, with the same managed-directive mechanism theming uses.

**The include replaces whole sections (D1).** A device section the shell writes replaces the
user's section for that device. So:
- **Unchanged devices.** The shell writes no section for a device until the user changes one of
  its settings on the page. Until then, the user's own section applies unchanged.
- **First write.** The shell first reads the user's current section for that device from the
  config and its includes, top level only. It copies across every setting the page doesn't show
  (for example `tap-button-map` and `drag-lock`), so nothing is lost. If the config cannot be
  read, the page shows an error and writes nothing.
- **What the page shows.** Its values come from that same reading: the user's section, overlaid
  by the shell's include.

The same file works under Niri (26.04 supports optional includes) and sysc-compositor. Neither
needs an IPC request; both live-reload included files.

### D6. sysc-shell native sysc-compositor support

**Detection.** One place in `internal/platform` answers "which compositor": sysc-compositor when
`SYSC_SOCKET` is set and `NIRI_SOCKET`'s file name starts with `sysc-compositor.`, Niri otherwise. Tranche 1's theming directory choice and this page both use
it. Version strings are not parsed.

**Wording.**
- Package description: "Desktop shell for Niri and sysc-compositor".
- README and onboarding name both.
- The AGENTS.md rule "Support Niri first. Other compositors require a separate approved design"
  is amended: sysc-compositor is supported through this design.

**Packaging.**
- `aur/sysc-shell` drops `depends=('niri')`.
- It adds `optdepends=('sysc-compositor: the sysc compositor' 'niri: the Niri compositor')`,
  because a dependency cannot be satisfied by either package unless both provide a common name
  (my call; tranche 3 owns the recipe change).

**What is not needed.** No IPC work: the shell already uses only niri-ipc requests that
sysc-compositor implements.

## Constraints

- sysc-compositor:
  - Go and cgo only, with no `.c` or `.h` files; the cgo preamble lives in a `.go` file;
  - `runtime.LockOSThread`, and libinput calls only on the compositor thread;
  - one named test per command.
- sysc-shell:
  - Go only, with no cgo;
  - `Registry.mu` rules;
  - bd in the primary checkout;
  - plans gitignored;
  - its commit gates (`go test -race ./...` is that repository's rule, not this one's).
- The page must work under Niri, which is the shell's default compositor today.

## Left to the builder

- **Shell KDL reading.** How sysc-shell reads a device section:
  - reuse its KDL handling if it has a parser;
  - otherwise use the `kdl-go` module sysc-compositor already pins, after checking sysc-shell's
    dependency rule (pin by version and record why).
- **Page layout and copy,** within the Settings design system.
- **Code layout.** Whether `scroll-factor` and the swipe sign live with the axis code or in a small
  input-settings type in `internal/wlr`.
