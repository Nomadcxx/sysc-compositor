package ipc

import "testing"

func TestCloseoutActionFieldsSurviveTheWire(t *testing.T) {
	for _, line := range []string{
		`{"Action":{"DoScreenTransition":{"delay_ms":250}}}`,
		`{"Action":{"DoScreenTransition":{"delay_ms":null}}}`,
		`{"Action":{"SetWindowWidth":{"id":7,"change":{"AdjustFixed":-20}}}}`,
		`{"Action":{"CenterWindow":{"id":7}}}`,
		`{"Action":{"SwitchPresetWindowWidth":{"id":7}}}`,
		`{"Action":{"SwitchPresetWindowWidthBack":{"id":7}}}`,
		`{"Action":{"ToggleWindowRuleOpacity":{"id":7}}}`,
	} {
		request, err := DecodeRequest([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		if got := string(EncodeRequest(request)); got != line {
			t.Errorf("wire=%s, want %s", got, line)
		}
	}
}

func TestCloseoutWireRejectsMalformedActionFields(t *testing.T) {
	for _, line := range []string{
		`{"Action":{"ToggleDebugTint":null}}`,
		`{"Action":{"DoScreenTransition":{"delay_ms":65536}}}`,
		`{"Action":{"DoScreenTransition":{"delay_ms":-1}}}`,
		`{"Action":{"DoScreenTransition":{"delay_ms":"250"}}}`,
		`{"Action":{"DoScreenTransition":{"delay_ms":null,"extra":1}}}`,
		`{"Action":{"SetWindowWidth":{}}}`,
		`{"Action":{"SetWindowWidth":{"change":{"SetFixed":2147483648}}}}`,
		`{"Action":{"SetColumnWidth":{"change":{"SetFixed":1.5}}}}`,
		`{"Action":{"CenterWindow":{"id":"7"}}}`,
		`{"Action":{"SwitchPresetWindowWidth":{"id":-1}}}`,
		`{"Action":{"SwitchPresetWindowWidthBack":{"id":7,"extra":1}}}`,
		`{"Action":{"ToggleWindowRuleOpacity":{"id":7,"extra":1}}}`,
	} {
		if _, err := DecodeRequest([]byte(line)); err != ErrParse {
			t.Errorf("accepted %s: %v", line, err)
		}
	}
	for _, name := range []string{"FocusColumnLeft", "ToggleDebugTint", "DebugToggleOpaqueRegions", "DebugToggleDamage"} {
		if _, err := DecodeRequest([]byte(`{"Action":{"` + name + `":{"extra":1}}}`)); err != ErrParse {
			t.Errorf("accepted extra field on %s", name)
		}
	}
}
