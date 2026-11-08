package wlr

import (
	"encoding/json"
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

func TestOutputActionEditsConfig(t *testing.T) {
	section := config.OutputConfig{Name: "DP-1", Off: true, Modeline: &config.Modeline{}}
	for _, raw := range []string{`"On"`, `{"CustomMode":{"mode":{"width":1280,"height":720,"refresh":60}}}`, `{"Scale":{"scale":{"Specific":1.5}}}`, `{"Transform":{"transform":"Flipped90"}}`, `{"Position":{"position":{"Specific":{"x":-100,"y":20}}}}`, `{"Vrr":{"vrr":true,"on_demand":true}}`} {
		var err error
		section, err = editOutputConfig(section, json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
	}
	if section.Off || section.Mode == nil || !section.Mode.Custom || section.Mode.Width != 1280 || section.Modeline != nil || section.Scale == nil || *section.Scale != 1.5 || section.Transform != config.TransformFlipped90 || section.Position == nil || section.Position.X != -100 || section.VRR == nil || !section.VRR.OnDemand {
		t.Fatalf("edited config = %+v", section)
	}
	for _, raw := range []string{`{"Mode":{"mode":"Automatic"}}`, `{"Scale":{"scale":"Automatic"}}`, `{"Position":{"position":"Automatic"}}`, `{"Vrr":{"vrr":false,"on_demand":false}}`} {
		var err error
		section, err = editOutputConfig(section, json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
	}
	if section.Mode != nil || section.Scale != nil || section.Position != nil || section.VRR != nil {
		t.Fatalf("automatic config = %+v", section)
	}
	for _, raw := range []string{`{"CustomMode":{"mode":{"width":1280,"height":720,"refresh":null}}}`, `{"CustomMode":{"mode":{"width":0,"height":720,"refresh":60}}}`, `{"Scale":{"scale":{"Specific":null}}}`, `{"Scale":{"scale":{"Specific":"big"}}}`, `{"wat":{}}`} {
		if _, err := editOutputConfig(section, json.RawMessage(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
