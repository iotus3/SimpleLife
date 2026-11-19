// Package assets holds the pictures the program carries inside its binary.
package assets

import (
	_ "embed"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/app/iconpng"
)

// Icon is the application's icon, the same file viva.toml names for packaging.
//
//go:embed icon.png
var Icon []byte

// WindowIcon decodes Icon once for the running window, which the taskbar and
// Alt+Tab read on Windows. A failure would be a bug in the embedded file, not a
// condition the program can survive: nothing else would draw an icon.
func WindowIcon() app.WindowIcon {
	icon, err := iconpng.DecodeFit(Icon, 512)
	if err != nil {
		panic("assets: the embedded icon is unusable: " + err.Error())
	}
	return icon
}
