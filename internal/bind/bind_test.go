package bind

import "testing"

func TestParseChord(t *testing.T) {
	got, err := ParseChord("Mod+Ctrl+Page_Down")
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "Page_Down" || got.Modifiers != Modifiers(ModifierCtrl) || !got.UsesMod {
		t.Fatalf("ParseChord() = %+v, want unresolved Mod and Ctrl+Page_Down", got)
	}
}

func TestChordResolveMod(t *testing.T) {
	chord, err := ParseChord("Mod+Ctrl+H")
	if err != nil {
		t.Fatal(err)
	}
	got := chord.ResolveMod(ModifierAlt)
	if got.UsesMod || got.Modifiers != Modifiers(ModifierAlt|ModifierCtrl) {
		t.Fatalf("ResolveMod() = %+v, want Alt+Ctrl", got)
	}
}

func TestResolveModKeyByBackend(t *testing.T) {
	tests := []struct {
		name, normal, nested string
		isNested             bool
		want                 Modifier
	}{
		{name: "native default", want: ModifierSuper},
		{name: "nested default", isNested: true, want: ModifierAlt},
		{name: "nested opposite of configured Alt", normal: "Alt", isNested: true, want: ModifierSuper},
		{name: "nested explicit", normal: "Alt", nested: "Ctrl", isNested: true, want: ModifierCtrl},
		{name: "native configured", normal: "Mod5", want: ModifierLevel3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveModKey(tt.normal, tt.nested, tt.isNested)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("ResolveModKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseAction(t *testing.T) {
	tests := []struct {
		name string
		args []string
		kind string
		want bool
	}{
		{name: "close", kind: ActionCloseWindow, want: true},
		{name: "spawn", args: []string{"kitty", "--class", "test"}, kind: ActionSpawn, want: true},
		{name: "spawn-sh", args: []string{"exec kitty"}, kind: ActionSpawnSh, want: true},
		{name: "workspace index", args: []string{"9"}, kind: ActionFocusWorkspace, want: true},
		{name: "unimplemented remains visible", kind: "toggle-overview", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := ParseAction(tt.kind, tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if ok != tt.want {
				t.Fatalf("ParseAction() supported = %v, want %v", ok, tt.want)
			}
			if ok && got.Name != tt.kind {
				t.Fatalf("ParseAction() = %+v, want action %q", got, tt.kind)
			}
			if tt.kind == ActionFocusWorkspace && got.WorkspaceIndex != 9 {
				t.Fatalf("workspace index = %d, want 9", got.WorkspaceIndex)
			}
		})
	}
}

func TestParseChordRejectsMalformedInput(t *testing.T) {
	for _, chord := range []string{"", "+H", "Mod+", "Mod++H", "Mod+Bogus+H", "Mod+Mod+H", "Mod+Ctrl"} {
		t.Run(chord, func(t *testing.T) {
			if _, err := ParseChord(chord); err == nil {
				t.Fatalf("ParseChord(%q) succeeded, want malformed bind error", chord)
			}
		})
	}
}

func TestParseActionRejectsBadArguments(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: ActionCloseWindow, args: []string{"1"}},
		{name: ActionSpawn},
		{name: ActionSpawnSh, args: []string{"sh", "-c", "false"}},
		{name: ActionFocusWorkspace, args: []string{"0"}},
		{name: ActionFocusWorkspace, args: []string{"256"}},
	} {
		if _, ok, err := ParseAction(tt.name, tt.args); !ok || err == nil {
			t.Errorf("ParseAction(%q, %v) = (%v, %v), want supported action with argument error", tt.name, tt.args, ok, err)
		}
	}
}
