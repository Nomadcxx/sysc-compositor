package wlr

import (
	"fmt"
	"testing"
	"time"
)

// TestIdleOutputStopsCommitting is the 62-commit-a-second idle output
// (sysc-1469): sysc_repaint_scene_output asked for the next frame after every
// one it handled, so an output with nothing to draw re-armed itself as soon as
// the previous frame finished and never went quiet.
//
// The frame listener counts every frame whose scene repaint ran, so a quiet
// output is one whose count stops growing. The samples are taken through Post
// because the count is written on the compositor thread.
func TestIdleOutputStopsCommitting(t *testing.T) {
	c := newNested(t)
	headless, err := addHeadlessTestOutput(c, 1920, 1080)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(headless.destroy)

	const settle = 250 * time.Millisecond
	sample := func() int {
		got := make(chan int, 1)
		if err := c.Post(func() { got <- c.scenesDrawn() }); err != nil {
			t.Errorf("Post: %v", err)
			return -1
		}
		return <-got
	}

	verdict := make(chan error, 1)
	go func() {
		// Let whatever the new output damaged settle before the first sample,
		// then hold the output idle for a second window.
		time.Sleep(settle)
		before := sample()
		time.Sleep(settle)
		after := sample()
		if after > before {
			verdict <- fmt.Errorf("an idle output committed %d frames in %s (count %d -> %d), want none",
				after-before, settle, before, after)
		} else {
			verdict <- nil
		}
		c.Terminate()
	}()

	guard := time.AfterFunc(30*time.Second, c.Terminate)
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	guard.Stop()
	if err := <-verdict; err != nil {
		t.Fatal(err)
	}
}
