package presentation

import "github.com/vivaui/viva/app"

// appConfig is the window a test opens: the same shape as the real one, so a
// layout problem shows up in the test too.
func appConfig() app.Config {
	return appConfigSize(windowWidth, windowHeight)
}

// appConfigSize opens a window of a given size. A test that needs a control far
// down a page asks for a taller window instead of scrolling: a headless window only
// offers the controls it painted.
func appConfigSize(width, height int) app.Config {
	return app.Config{
		Title:     "SimpleLife",
		Width:     width,
		Height:    height,
		MinWidth:  windowMinWidth,
		MinHeight: windowMinHeight,
	}
}
