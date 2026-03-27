package photo

import (
	"image"
	"math"

	"golang.org/x/image/draw"
)

// fit scales src to fit within maxW×maxH, preserving aspect ratio.
// Returns src unchanged if it already fits.
func fit(src image.Image, maxW, maxH int) image.Image {
	b := src.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW <= maxW && srcH <= maxH {
		return src
	}
	ratio := math.Min(float64(maxW)/float64(srcW), float64(maxH)/float64(srcH))
	dstW := max(1, int(float64(srcW)*ratio))
	dstH := max(1, int(float64(srcH)*ratio))
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

// applyOrientation rotates/flips img according to the EXIF orientation value.
func applyOrientation(img image.Image, orientation int) image.Image {
	switch orientation {
	case 2:
		return flipH(img)
	case 3:
		return rotate180(img)
	case 4:
		return flipV(img)
	case 5:
		return transpose(img)
	case 6:
		return rotate90CW(img)
	case 7:
		return transverse(img)
	case 8:
		return rotate90CCW(img)
	default:
		return img
	}
}

func newRGBA(img image.Image) *image.RGBA {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Src.Draw(dst, dst.Bounds(), img, b.Min)
	return dst
}

func flipH(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := newRGBA(img)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(w-1-x, y, src.At(x, y))
		}
	}
	return dst
}

func flipV(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := newRGBA(img)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(x, h-1-y, src.At(x, y))
		}
	}
	return dst
}

func rotate180(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := newRGBA(img)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(w-1-x, h-1-y, src.At(x, y))
		}
	}
	return dst
}

func rotate90CW(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := newRGBA(img)
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(h-1-y, x, src.At(x, y))
		}
	}
	return dst
}

func rotate90CCW(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := newRGBA(img)
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(y, w-1-x, src.At(x, y))
		}
	}
	return dst
}

// transpose: flip along top-left to bottom-right diagonal (orientation 5)
func transpose(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := newRGBA(img)
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(y, x, src.At(x, y))
		}
	}
	return dst
}

// transverse: flip along top-right to bottom-left diagonal (orientation 7)
func transverse(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := newRGBA(img)
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(h-1-y, w-1-x, src.At(x, y))
		}
	}
	return dst
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
