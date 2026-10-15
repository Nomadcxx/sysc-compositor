package wlr

import "testing"

func TestPointerGrabGeometry(t *testing.T) {
	tests := []struct {
		name             string
		grab             pointerGrab
		cursorX, cursorY float64
		want             pointerGeometry
	}{
		{
			name: "move",
			grab: pointerGrab{
				mode: grabMove, cursorX: 100, cursorY: 200,
				x: 10, y: 20, width: 100, height: 80,
			},
			cursorX: 130, cursorY: 250,
			want: pointerGeometry{x: 40, y: 70, width: 100, height: 80},
		},
		{
			name: "resize from bottom right",
			grab: pointerGrab{
				mode: grabResize, cursorX: 100, cursorY: 200,
				x: 10, y: 20, width: 100, height: 80,
				edges: edgeRight | edgeBottom,
			},
			cursorX: 130, cursorY: 250,
			want: pointerGeometry{x: 10, y: 20, width: 130, height: 130},
		},
		{
			name: "resize from top left keeps opposite corner fixed",
			grab: pointerGrab{
				mode: grabResize, cursorX: 100, cursorY: 200,
				x: 10, y: 20, width: 100, height: 80,
				edges: edgeLeft | edgeTop,
			},
			cursorX: 120, cursorY: 240,
			want: pointerGeometry{x: 30, y: 60, width: 80, height: 40},
		},
		{
			name: "resize clamps to one logical pixel",
			grab: pointerGrab{
				mode: grabResize, cursorX: 100, cursorY: 200,
				x: 10, y: 20, width: 100, height: 80,
				edges: edgeLeft | edgeTop,
			},
			cursorX: 300, cursorY: 400,
			want: pointerGeometry{x: 109, y: 99, width: 1, height: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.grab.geometry(tt.cursorX, tt.cursorY); got != tt.want {
				t.Fatalf("geometry() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
