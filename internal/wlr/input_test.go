package wlr

import (
	"testing"

	"github.com/Nomadcxx/sysc-compositor/internal/layout"
)

func TestPointerGrabResizeOptions(t *testing.T) {
	tests := []struct {
		name  string
		edges uint32
		want  layout.ResizeOptions
	}{
		{
			name:  "left",
			edges: edgeLeft,
			want:  layout.ResizeOptions{ResizeWidth: true, AnchorRight: true},
		},
		{
			name:  "right",
			edges: edgeRight,
			want:  layout.ResizeOptions{ResizeWidth: true},
		},
		{
			name:  "top",
			edges: edgeTop,
			want:  layout.ResizeOptions{ResizeHeight: true, AnchorBottom: true},
		},
		{
			name:  "bottom",
			edges: edgeBottom,
			want:  layout.ResizeOptions{ResizeHeight: true},
		},
		{
			name:  "top left corner",
			edges: edgeTop | edgeLeft,
			want: layout.ResizeOptions{
				ResizeWidth: true, ResizeHeight: true, AnchorRight: true, AnchorBottom: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (pointerGrab{edges: tt.edges}).resizeOptions(); got != tt.want {
				t.Fatalf("resizeOptions() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

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
