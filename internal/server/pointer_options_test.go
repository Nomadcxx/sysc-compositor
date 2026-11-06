package server

import (
	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"reflect"
	"testing"
)

func TestPointerOptionsForwardConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Input.Touchpad = config.PointerDevice{Set: true, Tap: true, NaturalScroll: true, ScrollFactor: config.ScrollFactor{Vertical: 2, Horizontal: 0.3}}
	cfg.Input.Mouse = config.PointerDevice{Set: true, LeftHanded: true, ScrollFactor: config.ScrollFactor{Vertical: 1, Horizontal: -1}}
	if got := compositorOptions(cfg, nil).Pointers; !reflect.DeepEqual(got, cfg.Input.Pointers) {
		t.Fatalf("pointer options=%+v want %+v", got, cfg.Input.Pointers)
	}
}
