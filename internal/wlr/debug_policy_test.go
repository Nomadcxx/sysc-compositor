package wlr

import (
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wm"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDebugDeviceSelectionUsesFileIdentity(t *testing.T) {
	dir := t.TempDir()
	first, second, alias := filepath.Join(dir, "first"), filepath.Join(dir, "second"), filepath.Join(dir, "alias")
	if err := os.WriteFile(first, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(first, alias); err != nil {
		t.Fatal(err)
	}
	got, err := filterDebugDRMDevices([]string{first, second}, []string{alias})
	if err != nil || !reflect.DeepEqual(got, []string{second}) {
		t.Fatalf("selection=%v err=%v", got, err)
	}
	if _, err = filterDebugDRMDevices([]string{first}, []string{alias}); err == nil {
		t.Fatal("all ignored GPUs silently fell back")
	}
}

func TestDebugActivationPolicyKeepsUrgencyAndSeatBoundaries(t *testing.T) {
	for _, tc := range []struct{ serial, valid, seat, honor, want bool }{
		{false, false, false, true, false}, {true, false, true, false, false},
		{true, false, true, true, true}, {true, true, true, false, true}, {true, true, false, true, false},
	} {
		if got := debugActivationMayFocus(tc.serial, tc.valid, tc.seat, tc.honor); got != tc.want {
			t.Fatalf("policy %+v=%t", tc, got)
		}
	}
}

func TestDebugActivationStateKeepsOtherOutputsAndTabbedColumns(t *testing.T) {
	one, two := uint64(1), uint64(3)
	snapshot := wm.Snapshot{Workspaces: []wm.WorkspaceSnapshot{{ID: 10, IsActive: true, ActiveWindowID: &one}, {ID: 20, IsActive: true, ActiveWindowID: &two}}, Windows: []wm.WindowSnapshot{
		{ID: 1, WorkspaceID: 10, Column: 0, Tile: layout.Placement{Display: layout.ColumnDisplayTabbed}},
		{ID: 2, WorkspaceID: 10, Column: 0, Tile: layout.Placement{Display: layout.ColumnDisplayTabbed}},
		{ID: 3, WorkspaceID: 20, Column: 0},
		{ID: 4, WorkspaceID: 30, Column: 0},
	}}
	normal := debugActivatedWindows(snapshot, 1, false)
	if !normal[1] || !normal[2] || !normal[3] || normal[4] {
		t.Fatalf("normal activation=%v", normal)
	}
	only := debugActivatedWindows(snapshot, 1, true)
	if !only[1] || only[2] || only[3] || only[4] {
		t.Fatalf("keyboard-only activation=%v", only)
	}
}
