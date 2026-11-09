package ui

import "testing"

func TestConfigNotificationScalesItsPanel(t *testing.T) {
	one, err := ConfigErrorNotification(1)
	if err != nil {
		t.Fatal(err)
	}
	two, err := ConfigErrorNotification(2)
	if err != nil {
		t.Fatal(err)
	}
	if one.RGBAAt(0, 0) != exitDialogBorderRGB || one.RGBAAt(8, 8) != exitDialogBackground {
		t.Error("notification lost its error border or background")
	}
	if d := two.Bounds().Dx() - 2*one.Bounds().Dx(); d < -5 || d > 5 {
		t.Errorf("scale2 dimensions=%v scale1=%v", two.Bounds(), one.Bounds())
	}
}
