package photo

import (
	"bytes"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

// EXIFData holds the fields we care about from a photo's EXIF metadata.
type EXIFData struct {
	Timestamp   *time.Time
	Lat         *float64
	Lng         *float64
	Orientation int // EXIF orientation tag (1–8); 0 means absent/unknown
}

// ExtractEXIF reads EXIF metadata from raw image bytes.
// Returns zero-value EXIFData fields for any data that cannot be read.
func ExtractEXIF(data []byte) EXIFData {
	var result EXIFData

	x, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		return result
	}

	if t, err := x.DateTime(); err == nil {
		result.Timestamp = &t
	}

	if lat, lng, err := x.LatLong(); err == nil {
		result.Lat = &lat
		result.Lng = &lng
	}

	if tag, err := x.Get(exif.Orientation); err == nil {
		if o, err := tag.Int(0); err == nil {
			result.Orientation = o
		}
	}

	return result
}
