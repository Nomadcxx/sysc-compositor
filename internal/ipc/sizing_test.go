package ipc

import (
	"reflect"
	"testing"
)

// The lines are what Niri 26.04's `niri msg` writes for the column width and
// window height actions.
func TestSizeActionsRoundTripNiriWire(t *testing.T) {
	windowID := uint64(7)
	for _, tc := range []struct {
		line string
		want Action
	}{
		{`{"Action":{"MaximizeColumn":{}}}`, Action{Name: "MaximizeColumn"}},
		{`{"Action":{"CenterColumn":{}}}`, Action{Name: "CenterColumn"}},
		{`{"Action":{"CenterVisibleColumns":{}}}`, Action{Name: "CenterVisibleColumns"}},
		{`{"Action":{"ExpandColumnToAvailableWidth":{}}}`, Action{Name: "ExpandColumnToAvailableWidth"}},
		{`{"Action":{"SwitchPresetColumnWidth":{}}}`, Action{Name: "SwitchPresetColumnWidth"}},
		{`{"Action":{"SwitchPresetColumnWidthBack":{}}}`, Action{Name: "SwitchPresetColumnWidthBack"}},
		{`{"Action":{"MaximizeWindowToEdges":{"id":null}}}`, Action{Name: "MaximizeWindowToEdges"}},
		{`{"Action":{"MaximizeWindowToEdges":{"id":7}}}`, Action{Name: "MaximizeWindowToEdges", WindowID: &windowID}},
		{`{"Action":{"ResetWindowHeight":{"id":null}}}`, Action{Name: "ResetWindowHeight"}},
		{`{"Action":{"SwitchPresetWindowHeight":{"id":null}}}`, Action{Name: "SwitchPresetWindowHeight"}},
		{`{"Action":{"SwitchPresetWindowHeightBack":{"id":null}}}`, Action{Name: "SwitchPresetWindowHeightBack"}},
		{`{"Action":{"SetColumnWidth":{"change":{"SetProportion":50.0}}}}`,
			Action{Name: "SetColumnWidth", Size: &SizeChange{Kind: SizeSetProportion, Value: 50}}},
		{`{"Action":{"SetColumnWidth":{"change":{"AdjustProportion":-10.0}}}}`,
			Action{Name: "SetColumnWidth", Size: &SizeChange{Kind: SizeAdjustProportion, Value: -10}}},
		{`{"Action":{"SetColumnWidth":{"change":{"SetFixed":640}}}}`,
			Action{Name: "SetColumnWidth", Size: &SizeChange{Kind: SizeSetFixed, Value: 640}}},
		{`{"Action":{"SetColumnWidth":{"change":{"AdjustFixed":-20}}}}`,
			Action{Name: "SetColumnWidth", Size: &SizeChange{Kind: SizeAdjustFixed, Value: -20}}},
		{`{"Action":{"SetWindowHeight":{"id":null,"change":{"SetProportion":30.0}}}}`,
			Action{Name: "SetWindowHeight", Size: &SizeChange{Kind: SizeSetProportion, Value: 30}}},
		{`{"Action":{"SetWindowHeight":{"id":7,"change":{"SetFixed":300}}}}`,
			Action{Name: "SetWindowHeight", WindowID: &windowID, Size: &SizeChange{Kind: SizeSetFixed, Value: 300}}},
	} {
		got, err := DecodeRequest([]byte(tc.line))
		if err != nil {
			t.Errorf("%s: %v", tc.line, err)
			continue
		}
		if got.Name != "Action" || got.Action == nil {
			t.Errorf("%s: got %+v, want an action", tc.line, got)
			continue
		}
		if !reflect.DeepEqual(*got.Action, tc.want) {
			t.Errorf("%s: got %+v, want %+v", tc.line, *got.Action, tc.want)
		}
	}
}

func TestSizeActionsRejectMalformedFields(t *testing.T) {
	for _, line := range []string{
		`{"Action":{"SetColumnWidth":{}}}`,
		`{"Action":{"SetColumnWidth":{"change":null}}}`,
		`{"Action":{"SetColumnWidth":{"change":{"Nope":1}}}}`,
		`{"Action":{"SetColumnWidth":{"change":{"SetProportion":"half"}}}}`,
		`{"Action":{"SetColumnWidth":{"change":{"SetProportion":50},"extra":1}}}`,
		`{"Action":{"SetWindowHeight":{"id":null}}}`,
		`{"Action":{"MaximizeColumn":{"extra":1}}}`,
		`{"Action":{"MaximizeWindowToEdges":{"id":"seven"}}}`,
	} {
		if _, err := DecodeRequest([]byte(line)); err != ErrParse {
			t.Errorf("%q: err = %v, want %v", line, err, ErrParse)
		}
	}
}
