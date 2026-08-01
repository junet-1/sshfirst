//go:build linux

package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"ssh-first/internal/storage"
)

var (
	traySSHIcon  = renderTrayProtocolIcon(storage.HostProtocolSSH)
	traySFTPIcon = renderTrayProtocolIcon(storage.HostProtocolSFTP)
)

func trayProtocolIcon(protocol storage.HostProtocol) []byte {
	if protocol == storage.HostProtocolSFTP {
		return traySFTPIcon
	}
	return traySSHIcon
}

// renderTrayProtocolIcon produces compact PNGs for DBusMenu's icon-data
// property. Keeping them in code makes the tray independent of whichever icon
// theme happens to be installed on the desktop.
func renderTrayProtocolIcon(protocol storage.HostProtocol) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 24, 24))
	if protocol == storage.HostProtocolSFTP {
		drawSFTPIcon(img)
	} else {
		drawSSHIcon(img)
	}

	var data bytes.Buffer
	_ = png.Encode(&data, img)
	return data.Bytes()
}

func drawSSHIcon(img *image.NRGBA) {
	green := color.NRGBA{R: 54, G: 198, B: 116, A: 255}
	softGreen := color.NRGBA{R: 54, G: 198, B: 116, A: 72}
	dark := color.NRGBA{R: 21, G: 31, B: 27, A: 235}

	// Shackle.
	fillTrayRect(img, 8, 3, 16, 5, green)
	fillTrayRect(img, 6, 5, 9, 11, green)
	fillTrayRect(img, 15, 5, 18, 11, green)
	fillTrayRect(img, 9, 5, 15, 7, softGreen)

	// Lock body and keyhole.
	fillTrayRect(img, 4, 10, 20, 21, green)
	fillTrayRect(img, 6, 12, 18, 19, dark)
	fillTrayRect(img, 11, 14, 13, 18, green)
}

func drawSFTPIcon(img *image.NRGBA) {
	blue := color.NRGBA{R: 75, G: 154, B: 245, A: 255}
	softBlue := color.NRGBA{R: 75, G: 154, B: 245, A: 94}
	light := color.NRGBA{R: 239, G: 246, B: 255, A: 255}

	// Folder silhouette.
	fillTrayRect(img, 3, 6, 10, 9, blue)
	fillTrayRect(img, 3, 8, 21, 20, blue)
	fillTrayRect(img, 5, 10, 19, 18, softBlue)

	// Bidirectional transfer arrows.
	fillTrayRect(img, 7, 11, 16, 13, light)
	fillTrayRect(img, 14, 10, 18, 14, light)
	fillTrayRect(img, 8, 15, 17, 17, light)
	fillTrayRect(img, 6, 14, 10, 18, light)
}

func fillTrayRect(img *image.NRGBA, x0, y0, x1, y1 int, colour color.NRGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetNRGBA(x, y, colour)
		}
	}
}
