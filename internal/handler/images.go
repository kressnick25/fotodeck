package handler

import (
	"fotodeck/internal/images"
	"log/slog"
	"net/http"
)

type ImageHandler struct {
	FileEntries map[string]images.ImageFile
}

func (ih *ImageHandler) Previews(w http.ResponseWriter, r *http.Request) {
	requestFile := r.PathValue("id")
	entry, ok := ih.FileEntries[requestFile]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	slog.Debug("", "requestFile", "/img/preview/"+requestFile, "responseFile", entry.GetPreview())

	http.ServeFile(w, r, entry.GetPreview())
}

func (ih *ImageHandler) Images(w http.ResponseWriter, r *http.Request) {
	requestFile := r.PathValue("id")
	entry, ok := ih.FileEntries[requestFile]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	slog.Debug("", "requestFile", "/img/"+requestFile, "responseFile", entry.GetFullSize())

	http.ServeFile(w, r, entry.GetFullSize())
}
