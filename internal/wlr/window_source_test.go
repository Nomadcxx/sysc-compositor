package wlr

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/Nomadcxx/sysc-compositor/internal/config"
	"github.com/Nomadcxx/sysc-compositor/internal/layout"
	"github.com/Nomadcxx/sysc-compositor/internal/wlr/testclient"
)

func TestWindowCaptureTracksDamageResizeAndUnmap(t *testing.T) {
	c := newNested(t)
	done := make(chan error, 1)
	guard := time.AfterFunc(25*time.Second, c.Terminate)
	defer guard.Stop()
	go func() {
		defer c.Terminate()
		done <- func() error {
			window, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
			if err != nil {
				return err
			}
			defer func() {
				if window != nil {
					window.Close()
				}
			}()
			if err = window.SetIdentity("preview", "preview"); err != nil {
				return err
			}
			if err = window.Paint(0xff3498db, 200, 150); err != nil {
				return err
			}
			var id uint64
			onCompositor(t, c, func() {
				for _, v := range c.views {
					if cString(v.toplevel.app_id) == "preview" {
						id = v.id
					}
				}
			})
			client, err := testclient.StartCapture(c.SocketName(), strconv.FormatUint(id, 10))
			if err != nil {
				return err
			}
			defer client.Close()
			if state := client.State(); state.Width != 200 || state.Height != 150 || state.Constraints != 1 || state.Stopped != 0 {
				return fmt.Errorf("initial constraints: %+v", state)
			}
			// A second observer must release its source after destroying protocol objects,
			// even while this mapping and the first capture session stay alive.
			observer, e := testclient.StartCapture(c.SocketName(), strconv.FormatUint(id, 10))
			if e != nil {
				return e
			}
			if e = observer.DestroyObjects(); e != nil {
				observer.Close()
				return e
			}
			observer.Close()
			onCompositor(t, c, func() {
				if n := len(c.capture.sources); n != 1 {
					t.Errorf("destroyed observer retained %d sources, want 1", n)
				}
			})
			if err = client.Arm(false); err != nil {
				return err
			}
			if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 1 }); err != nil {
				return err
			}
			if got := client.Image().NRGBAAt(20, 20); got != (color.NRGBA{0x34, 0x98, 0xdb, 255}) {
				return fmt.Errorf("window image pixel %v", got)
			}
			if err = client.Arm(false); err != nil {
				return err
			}
			if err = client.Sync(); err != nil {
				return err
			}
			if err = client.Wait(80 * time.Millisecond); err != nil {
				return err
			}
			if state := client.State(); state.Ready != 1 {
				return fmt.Errorf("unchanged source delivered frame: %+v", state)
			}
			if err = window.Paint(0xffc0392b, 200, 150); err != nil {
				return err
			}
			if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 2 }); err != nil {
				return err
			}
			if got := client.Image().NRGBAAt(20, 20); got != (color.NRGBA{0xc0, 0x39, 0x2b, 255}) {
				return fmt.Errorf("damaged image pixel %v", got)
			}
			if err = client.Arm(false); err != nil {
				return err
			}
			if err = window.PaintSubsurface(0xffffcc00, 15, 15, 40, 40); err != nil {
				return err
			}
			if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 3 }); err != nil {
				return err
			}
			if got := client.Image().NRGBAAt(20, 20); got != (color.NRGBA{255, 204, 0, 255}) {
				return fmt.Errorf("subsurface damage pixel %v", got)
			}
			if err = window.Paint(0xff228844, 240, 180); err != nil {
				return err
			}
			if err = client.Sync(); err != nil {
				return err
			}
			if state := client.State(); state.Width != 240 || state.Height != 180 || state.Constraints < 2 {
				return fmt.Errorf("resize constraints: %+v", state)
			}
			if err = client.Arm(false); err != nil {
				return err
			}
			if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 4 }); err != nil {
				return err
			}
			if err = client.Arm(true); err != nil {
				return err
			}
			if err = window.CommitBuffer(); err != nil {
				return err
			}
			if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Failed == 1 }); err != nil {
				return err
			}
			// Retain the capture source and frame while the mapping disappears.
			window.Close()
			window = nil
			if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Stopped == 1 }); err != nil {
				return err
			}
			if err = client.Arm(false); err != nil {
				return err
			}
			if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Failed >= 2 }); err != nil {
				return err
			}
			if err = client.Reopen(); err != nil {
				return err
			}
			if state := client.State(); state.Stopped != 2 {
				return fmt.Errorf("stale foreign handle opened active source: %+v", state)
			}
			return nil
		}()
	}()
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("window capture check did not finish")
	}
}

func waitCapture(c *testclient.CaptureClient, ready func(testclient.CaptureState) bool) error {
	deadline := time.Now().Add(3 * time.Second)
	for !ready(c.State()) {
		if time.Now().After(deadline) {
			return fmt.Errorf("capture checkpoint timed out: %+v", c.State())
		}
		if err := c.Wait(100 * time.Millisecond); err != nil {
			return err
		}
	}
	return nil
}

func TestCapturePointerMetadataAndImageLifetime(t *testing.T) {
	floating := true
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}, WindowRules: []config.WindowRule{{OpenFloating: &floating}}})
	runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
		window, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() {
			if window != nil {
				window.Close()
			}
		}()
		if err = window.Paint(0xff3498db, 200, 150); err != nil {
			t.Error(err)
			return
		}
		var pointer *fakePointer
		defer onCompositor(t, c, func() {
			if pointer != nil {
				pointer.remove()
			}
		})
		var id uint64
		var at, hotspot image.Point
		var want *image.RGBA
		onCompositor(t, c, func() {
			var v *toplevelView
			for _, candidate := range c.views {
				v = candidate
				break
			}
			if v == nil {
				t.Error("window missing")
				return
			}
			id = v.id
			x, y := v.treePosition()
			at = image.Pt(x+30, y+30)
			pointer = c.addFakePointer()
			for output := range c.sceneOutputs {
				box := c.outputLayoutBox(output)
				pointer.moveTo(float64(box.Max.X-2), float64(box.Max.Y-2))
				break
			}
			var ok bool
			want, hotspot, ok = c.pointerImage(c.windowCaptureScale(v))
			if !ok {
				t.Error("no pointer image")
			}
		})
		if want == nil {
			return
		}
		client, err := testclient.StartCapture(c.SocketName(), strconv.FormatUint(id, 10))
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		if err = client.StartPointerCapture(); err != nil {
			t.Error(err)
			return
		}
		state := client.State()
		position := client.PointerState()
		if position.Entered || state.Stopped != 0 || state.Width != 1 || state.Height != 1 {
			t.Errorf("outside pointer source was not kept alive: %+v %+v", state, position)
			return
		}
		onCompositor(t, c, func() { pointer.moveTo(float64(at.X), float64(at.Y)) })
		if err = waitCapture(client, func(s testclient.CaptureState) bool {
			return client.PointerState().Entered && s.Width == want.Bounds().Dx() && s.Height == want.Bounds().Dy()
		}); err != nil {
			t.Error(err)
			return
		}
		state, position = client.State(), client.PointerState()
		if !position.Entered || state.Stopped != 0 || state.Width != want.Bounds().Dx() || state.Height != want.Bounds().Dy() {
			t.Errorf("pointer capture constraints %+v metadata %+v", state, position)
			return
		}
		if position.HotspotX != hotspot.X || position.HotspotY != hotspot.Y {
			t.Errorf("hotspot %+v want %v", position, hotspot)
			return
		}
		if err = client.Arm(false); err != nil {
			t.Error(err)
			return
		}
		if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 1 }); err != nil {
			t.Error(err)
			return
		}
		got := client.Image()
		for y := 0; y < want.Bounds().Dy(); y++ {
			for x := 0; x < want.Bounds().Dx(); x++ {
				expected := color.NRGBAModel.Convert(want.At(x, y)).(color.NRGBA)
				if actual := got.NRGBAAt(x, y); actual != expected {
					t.Errorf("pointer pixel(%d,%d) %v want %v", x, y, actual, expected)
					return
				}
			}
		}
		// Destroying the factory's raw source cannot end the cursor/image sessions.
		if err = client.DropRawSource(); err != nil {
			t.Error(err)
			return
		}
		if err = client.Arm(false); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() { pointer.moveTo(float64(at.X+10), float64(at.Y+15)) })
		if err = waitCapture(client, func(_ testclient.CaptureState) bool {
			moved := client.PointerState()
			return moved.X-position.X == 10 && moved.Y-position.Y == 15
		}); err != nil {
			t.Error(err)
			return
		}
		moved := client.PointerState()
		if moved.X-position.X != 10 || moved.Y-position.Y != 15 || !moved.Entered {
			t.Errorf("pointer position did not follow motion: before=%+v after=%+v", position, moved)
			return
		}
		if client.State().Ready != 1 {
			t.Error("unchanged pointer image produced another frame")
			return
		}
		// Enter follows the image rectangle, even when its hotspot is outside.
		onCompositor(t, c, func() {
			x := at.X - 30 - (want.Bounds().Dx() - hotspot.X) + 1
			pointer.moveTo(float64(x), float64(at.Y))
		})
		if err = waitCapture(client, func(_ testclient.CaptureState) bool { return client.PointerState().X < 0 }); err != nil {
			t.Error(err)
			return
		}
		if edge := client.PointerState(); !edge.Entered || edge.Leaves != 0 {
			t.Errorf("partially visible pointer left early: %+v", edge)
			return
		}
		onCompositor(t, c, func() {
			for output := range c.sceneOutputs {
				box := c.outputLayoutBox(output)
				pointer.moveTo(float64(box.Max.X-2), float64(box.Max.Y-2))
				break
			}
		})
		if err = waitCapture(client, func(_ testclient.CaptureState) bool { return !client.PointerState().Entered }); err != nil {
			t.Error(err)
			return
		}
		if position = client.PointerState(); position.Entered || position.Leaves != 1 {
			t.Errorf("pointer failed to leave capture source: %+v", position)
			return
		}
		// A named icon change must work without any pointer motion afterwards.
		onCompositor(t, c, func() { pointer.moveTo(float64(at.X), float64(at.Y)) })
		if err = waitCapture(client, func(_ testclient.CaptureState) bool { return client.PointerState().Entered }); err != nil {
			t.Error(err)
			return
		}
		still := client.PointerState()
		var changed *image.RGBA
		var changedHotspot image.Point
		onCompositor(t, c, func() {
			c.showCursorImage("crosshair")
			for _, v := range c.views {
				changed, changedHotspot, _ = c.pointerImage(c.windowCaptureScale(v))
				break
			}
		})
		if changed == nil {
			t.Error("crosshair pointer image unavailable")
			return
		}
		// The pointer update was queued before this compositor checkpoint.
		onCompositor(t, c, func() {})
		if err = client.Sync(); err != nil {
			t.Error(err)
			return
		}
		if client.State().Ready == 1 {
			if err = client.Arm(false); err != nil {
				t.Error(err)
				return
			}
			if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 2 }); err != nil {
				t.Error(err)
				return
			}
		}
		if now := client.PointerState(); now.X != still.X || now.Y != still.Y || now.HotspotX != changedHotspot.X || now.HotspotY != changedHotspot.Y {
			t.Errorf("stationary icon metadata %+v want hotspot %v at %+v", now, changedHotspot, still)
			return
		}
		got = client.Image()
		if got.Bounds() != changed.Bounds() {
			t.Errorf("changed icon dimensions %v want %v", got.Bounds(), changed.Bounds())
			return
		}
		for y := 0; y < changed.Bounds().Dy(); y++ {
			for x := 0; x < changed.Bounds().Dx(); x++ {
				if actual, expected := got.NRGBAAt(x, y), color.NRGBAModel.Convert(changed.At(x, y)).(color.NRGBA); actual != expected {
					t.Errorf("stationary icon pixel(%d,%d) %v want %v", x, y, actual, expected)
					return
				}
			}
		}
		window.Close()
		window = nil
		if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Stopped == 1 }); err != nil {
			t.Error(err)
		}
	})
}

func TestCaptureRegionClipsUntrustedCoordinates(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 80)
	for _, tc := range []struct {
		x, y, w, h int32
		want       image.Rectangle
	}{
		{10, 20, 30, 40, image.Rect(10, 20, 40, 60)},
		{-10, -20, 30, 40, image.Rect(0, 0, 20, 20)},
		{90, 70, 30, 40, image.Rect(90, 70, 100, 80)},
		{-10, -10, math.MaxInt32, math.MaxInt32, bounds},
		{math.MaxInt32, 0, math.MaxInt32, 10, image.Rectangle{}},
		{math.MinInt32, 0, math.MaxInt32, 10, image.Rectangle{}},
		{0, 0, -1, 10, image.Rectangle{}},
		{0, 0, 10, 0, image.Rectangle{}},
	} {
		got, valid := clipCaptureRegion(tc.x, tc.y, tc.w, tc.h, 1, bounds)
		if got != tc.want || valid != !tc.want.Empty() {
			t.Errorf("region %d,%d %dx%d = %v,%t want %v", tc.x, tc.y, tc.w, tc.h, got, valid, tc.want)
		}
	}
}

func TestLegacyScreencopyPrivacyRegionAndDamage(t *testing.T) {
	floating := true
	c := newNestedWith(t, Options{Animations: config.Animations{Off: true}, WindowRules: []config.WindowRule{{OpenFloating: &floating}}})
	runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
		window, err := testclient.StartToplevelClient(c.SocketName(), time.Hour)
		if err != nil {
			t.Error(err)
			return
		}
		defer window.Close()
		if err = window.SetIdentity("legacy", "legacy"); err != nil {
			t.Error(err)
			return
		}
		if err = window.Paint(0xff3498db, 200, 150); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() {
			for _, view := range c.views {
				c.MoveFloatingWindow(&view.id, layout.PositionChange{Kind: layout.PositionSetFixed, Value: 40}, layout.PositionChange{Kind: layout.PositionSetFixed, Value: 40})
			}
		})
		client, err := testclient.StartLegacyCapture(c.SocketName(), nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		if err = client.ArmLegacy(false, false); err != nil {
			t.Error(err)
			return
		}
		if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 1 }); err != nil {
			t.Error(err)
			return
		}
		if got := client.Image().NRGBAAt(60, 60); got != (color.NRGBA{0x34, 0x98, 0xdb, 255}) {
			t.Errorf("legacy pixel %v", got)
			return
		}
		if err = client.ArmLegacy(true, false); err != nil {
			t.Error(err)
			return
		}
		if err = client.Sync(); err != nil {
			t.Error(err)
			return
		}
		if err = client.Wait(80 * time.Millisecond); err != nil {
			t.Error(err)
			return
		}
		if state := client.State(); state.Ready != 1 {
			t.Errorf("undamaged legacy frame delivered: %+v", state)
			return
		}
		if err = window.Paint(0xffc0392b, 200, 150); err != nil {
			t.Error(err)
			return
		}
		if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 2 }); err != nil {
			t.Error(err)
			return
		}
		if state := client.State(); state.Damage != 1 {
			t.Errorf("legacy damage event: %+v", state)
			return
		}
		if got := client.Image().NRGBAAt(60, 60); got != (color.NRGBA{0xc0, 0x39, 0x2b, 255}) {
			t.Errorf("damaged legacy pixel %v", got)
			return
		}
		region, err := testclient.StartLegacyCapture(c.SocketName(), &[4]int32{-10, -20, 80, 100})
		if err != nil {
			t.Error(err)
			return
		}
		defer region.Close()
		if state := region.State(); state.Width != 70 || state.Height != 80 || state.Failed != 0 {
			t.Errorf("clipped legacy constraints %+v", state)
			return
		}
		if err = region.ArmLegacy(false, false); err != nil {
			t.Error(err)
			return
		}
		if err = waitCapture(region, func(s testclient.CaptureState) bool { return s.Ready == 1 }); err != nil {
			t.Error(err)
			return
		}
		if got := region.Image().NRGBAAt(60, 60); got != (color.NRGBA{0xc0, 0x39, 0x2b, 255}) {
			t.Errorf("clipped legacy pixel %v", got)
			return
		}
		invalid, err := testclient.StartLegacyCapture(c.SocketName(), &[4]int32{math.MaxInt32, 0, math.MaxInt32, 10})
		if err != nil {
			t.Error(err)
			return
		}
		defer invalid.Close()
		if state := invalid.State(); state.Failed != 1 || state.Constraints != 0 {
			t.Errorf("invalid region %+v", state)
			return
		}
		malformed, err := testclient.StartLegacyCapture(c.SocketName(), nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer malformed.Close()
		if err = malformed.ArmLegacy(false, true); err != nil {
			t.Error(err)
			return
		}
		if err = malformed.Sync(); err == nil {
			t.Error("legacy accepted a stride different from its constraints")
			return
		}
		if iface, code := malformed.ProtocolError(); iface != "zwlr_screencopy_frame_v1" || code != 0 {
			t.Errorf("wrong malformed-buffer error %s:%d", iface, code)
			return
		}
		block := "screen-capture"
		onCompositor(t, c, func() {
			err = c.Reconfigure(Options{Animations: config.Animations{Off: true}, WindowRules: []config.WindowRule{{Matches: []config.RuleMatch{{AppID: regexp.MustCompile("^legacy$")}}, BlockOutFrom: &block}}})
		})
		if err != nil {
			t.Error(err)
			return
		}
		if err = client.ArmLegacy(false, false); err != nil {
			t.Error(err)
			return
		}
		if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 3 }); err != nil {
			t.Error(err)
			return
		}
		if got := client.Image().NRGBAAt(60, 60); got != (color.NRGBA{A: 255}) {
			t.Errorf("legacy privacy pixel %v", got)
			return
		}
		if err = client.ArmLegacy(true, false); err != nil {
			t.Error(err)
			return
		}
		if err = client.Sync(); err != nil {
			t.Error(err)
			return
		}
		onCompositor(t, c, func() { err = c.PowerOffMonitors() })
		defer onCompositor(t, c, func() { _ = c.PowerOnMonitors() })
		if err != nil {
			t.Error(err)
			return
		}
		if err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Failed == 1 }); err != nil {
			t.Error(err)
			return
		}
	})
}

func TestCaptureRegionScalesBeforeClipping(t *testing.T) {
	bounds := image.Rect(0, 0, 101, 79)
	for _, tc := range []struct {
		name       string
		x, y, w, h int32
		scale      float64
		want       image.Rectangle
	}{
		{"round extent", 0, 0, 1, 1, 1.5, image.Rect(0, 0, 2, 2)},
		{"round endpoints", 1, 1, 1, 1, 1.5, image.Rect(2, 2, 3, 3)},
		{"cover fractional edge", 0, 0, 68, 53, 1.5, bounds},
		{"clip scaled negative origin", -1, -1, 2, 2, 1.5, image.Rect(0, 0, 2, 2)},
		{"last physical pixels", 66, 52, 2, 1, 1.5, image.Rect(99, 78, 101, 79)},
		{"rounded beyond edge", 67, 52, 1, 1, 1.5, image.Rectangle{}},
		{"large request", -1, -1, math.MaxInt32, math.MaxInt32, 1.5, bounds},
		{"outside", math.MaxInt32, 0, math.MaxInt32, 1, 1.5, image.Rectangle{}},
		{"rounded empty", 0, 0, 1, 1, 0.1, image.Rectangle{}},
		{"zero scale", 0, 0, 1, 1, 0, image.Rectangle{}},
		{"negative scale", 0, 0, 1, 1, -1, image.Rectangle{}},
		{"nan scale", 0, 0, 1, 1, math.NaN(), image.Rectangle{}},
		{"infinite scale", 0, 0, 1, 1, math.Inf(1), image.Rectangle{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, valid := clipCaptureRegion(tc.x, tc.y, tc.w, tc.h, tc.scale, bounds)
			if got != tc.want || valid != !tc.want.Empty() {
				t.Errorf("got %v,%t, want %v", got, valid, tc.want)
			}
		})
	}
}

func TestLegacyScreencopyFractionalRegionPreservesPhysicalEdges(t *testing.T) {
	c := newNested(t)
	if err := c.setOutputScale(1.5); err != nil {
		t.Fatal(err)
	}
	runTools(t, c, func(_ func(string, ...string) *exec.Cmd) {
		lock, err := testclient.StartLockClient(c.SocketName(), true)
		if err != nil {
			t.Error(err)
			return
		}
		defer lock.Close()
		if err = lock.Paint(0xff3498db); err != nil {
			t.Error(err)
			return
		}
		full, err := testclient.StartLegacyCapture(c.SocketName(), nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer full.Close()
		if err = full.ArmLegacy(false, false); err != nil {
			t.Error(err)
			return
		}
		if err = waitCapture(full, func(s testclient.CaptureState) bool { return s.Ready == 1 }); err != nil {
			t.Error(err)
			return
		}
		size := full.State()
		for _, tc := range []struct {
			region [4]int32
			box    image.Rectangle
		}{
			{[4]int32{0, 0, 1, 1}, image.Rect(0, 0, 2, 2)},
			{[4]int32{1, 1, 1, 1}, image.Rect(2, 2, 3, 3)},
			{[4]int32{0, 0, int32(math.Ceil(float64(size.Width) / 1.5)), int32(math.Ceil(float64(size.Height) / 1.5))}, image.Rect(0, 0, size.Width, size.Height)},
		} {
			region, box := tc.region, tc.box
			client, err := testclient.StartLegacyCapture(c.SocketName(), &region)
			if err != nil {
				t.Error(err)
				return
			}
			if state := client.State(); state.Width != box.Dx() || state.Height != box.Dy() || state.Failed != 0 {
				t.Errorf("fractional region %v constraints %+v want %v", region, state, box.Size())
				client.Close()
				return
			}
			if err = client.ArmLegacy(false, false); err == nil {
				err = waitCapture(client, func(s testclient.CaptureState) bool { return s.Ready == 1 })
			}
			if err != nil {
				t.Error(err)
				client.Close()
				return
			}
			for _, at := range []image.Point{{0, 0}, box.Size().Sub(image.Pt(1, 1))} {
				got := client.Image().NRGBAAt(at.X, at.Y)
				want := full.Image().NRGBAAt(at.X+box.Min.X, at.Y+box.Min.Y)
				if got != want {
					t.Errorf("fractional region %v pixel %v=%v want %v", region, at, got, want)
				}
			}
			client.Close()
		}
	})
}
