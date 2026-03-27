package photo

import (
	"image"
	"math"

	"golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
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
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

// applyOrientation rotates/flips img according to the EXIF orientation value.
func applyOrientation(img image.Image, orientation int) image.Image {
	switch orientation {
	case 2:
		return transform(img, img.Bounds().Dx(), img.Bounds().Dy(),
			f64.Aff3{-1, 0, float64(img.Bounds().Dx() - 1), 0, 1, 0})
	case 3:
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		return transform(img, w, h,
			f64.Aff3{-1, 0, float64(w - 1), 0, -1, float64(h - 1)})
	case 4:
		return transform(img, img.Bounds().Dx(), img.Bounds().Dy(),
			f64.Aff3{1, 0, 0, 0, -1, float64(img.Bounds().Dy() - 1)})
	case 5: // transpose
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		return transform(img, h, w, f64.Aff3{0, 1, 0, 1, 0, 0})
	case 6: // rotate 90° CW
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		return transform(img, h, w,
			f64.Aff3{0, -1, float64(h - 1), 1, 0, 0})
	case 7: // transverse
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		return transform(img, h, w,
			f64.Aff3{0, -1, float64(h - 1), -1, 0, float64(w - 1)})
	case 8: // rotate 90° CCW
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		return transform(img, h, w,
			f64.Aff3{0, 1, 0, -1, 0, float64(w - 1)})
	default:
		return img
	}
}

// transform applies an affine transformation (source→destination mapping) using
// NearestNeighbor — lossless for axis-aligned flips and 90° rotations.
func transform(src image.Image, dstW, dstH int, s2d f64.Aff3) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.NearestNeighbor.Transform(dst, s2d, src, src.Bounds(), draw.Src, nil)
	return dst
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
