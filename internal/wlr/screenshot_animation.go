package wlr

var screenshotOpenKey = animationKey{property: "screenshot-ui-open"}

func (c *Compositor) animateScreenshotOpen(shot *screenshotUI) {
	c.cancelAnimation(screenshotOpenKey)
	c.animateValue(screenshotOpenKey, c.animationConfig.ScreenshotUIOpen, 0, 1, 0, func(progress float64) {
		if c.shotUI != shot {
			return
		}
		shot.openProgress = clampOpacity(progress)
		c.drawShotSelection()
		for _, frame := range shot.frames {
			c.syncShotHint(frame)
		}
	}, nil)
}
