package photo

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/patrikhson/french75/internal/middleware"
	_ "golang.org/x/image/webp"
)

const maxUploadSize = 50 << 20 // 50 MB

type Handler struct {
	db      *pgxpool.Pool
	storage Storage
}

func NewHandler(db *pgxpool.Pool, storage Storage) *Handler {
	return &Handler{db: db, storage: storage}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("POST /photos/upload", requireAuth(http.HandlerFunc(h.upload)))
}

type uploadResponse struct {
	ID           string   `json:"id"`
	URL          string   `json:"url"`
	ThumbnailURL string   `json:"thumbnail_url"`
	ExifTime     *string  `json:"exif_timestamp,omitempty"`
	ExifLat      *float64 `json:"exif_lat,omitempty"`
	ExifLng      *float64 `json:"exif_lng,omitempty"`
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "File too large (max 50 MB)", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("photo")
	if err != nil {
		http.Error(w, "photo field required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	var buf bytes.Buffer
	buf.ReadFrom(file)
	data := buf.Bytes()

	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/jpeg") && !strings.HasPrefix(mime, "image/png") && !strings.HasPrefix(mime, "image/webp") {
		http.Error(w, "Unsupported image format (got "+mime+"). Please use JPEG, PNG, or WebP.", http.StatusBadRequest)
		return
	}

	exifData := ExtractEXIF(data)

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		http.Error(w, "Could not decode image", http.StatusBadRequest)
		return
	}
	src = applyOrientation(src, exifData.Orientation)

	resized := fit(src, 2000, 2000)
	thumb := fit(src, 400, 400)

	id := uuid.NewString()
	mainFilename := id + ".jpg"
	thumbFilename := id + "_thumb.jpg"

	var mainBuf, thumbBuf bytes.Buffer
	jpeg.Encode(&mainBuf, resized, &jpeg.Options{Quality: 85})
	jpeg.Encode(&thumbBuf, thumb, &jpeg.Options{Quality: 80})

	mainPath, err := h.storage.Save(r.Context(), &mainBuf, mainFilename)
	if err != nil {
		http.Error(w, "Could not save photo", http.StatusInternalServerError)
		return
	}
	thumbPath, err := h.storage.Save(r.Context(), &thumbBuf, thumbFilename)
	if err != nil {
		http.Error(w, "Could not save thumbnail", http.StatusInternalServerError)
		return
	}

	userID := middleware.GetUserID(r)
	bounds := resized.Bounds()

	_, err = h.db.Exec(r.Context(),
		`INSERT INTO photos (id, user_id, storage_path, thumbnail_path, mime_type,
		                     size_bytes, width_px, height_px,
		                     exif_timestamp, exif_gps_lat, exif_gps_lng)
		 VALUES ($1, $2, $3, $4, 'image/jpeg', $5, $6, $7, $8, $9, $10)`,
		id, userID, mainPath, thumbPath,
		len(data),
		bounds.Dx(), bounds.Dy(),
		exifData.Timestamp, exifData.Lat, exifData.Lng,
	)
	if err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	resp := uploadResponse{
		ID:           id,
		URL:          h.storage.URL(mainPath),
		ThumbnailURL: h.storage.URL(thumbPath),
	}
	if exifData.Timestamp != nil {
		s := exifData.Timestamp.UTC().Format("2006-01-02T15:04:05Z")
		resp.ExifTime = &s
	}
	resp.ExifLat = exifData.Lat
	resp.ExifLng = exifData.Lng

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
