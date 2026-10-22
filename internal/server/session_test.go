package server

import (
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
)

func TestLocaleKeyboardFillsOnlyUnsetXKBSettings(t *testing.T) {
	locale := config.Keyboard{Model: "pc105", Layout: "us,de", Variant: "", Options: "grp:alt_shift_toggle"}
	configured := config.Keyboard{RepeatDelay: 400, RepeatRate: 40}
	got := keyboardWithLocale(configured, locale)
	if got.Model != locale.Model || got.Layout != locale.Layout || got.Options != locale.Options ||
		got.RepeatDelay != configured.RepeatDelay || got.RepeatRate != configured.RepeatRate {
		t.Fatalf("locale keyboard overlay = %+v, want XKB defaults and configured repeat settings", got)
	}
	configured.Layout = "de"
	got = keyboardWithLocale(configured, locale)
	if got.Layout != "de" || got.Model != "" || got.Options != "" {
		t.Fatalf("explicit KDL XKB settings were overridden: %+v", got)
	}
}
