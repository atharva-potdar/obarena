package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/google/uuid"
)

var allowedLanguages = map[string]bool{
	"cpp":  true,
	"rust": true,
	"go":   true,
}

type Handler struct {
	storage   *Storage
	publisher *Publisher
	maxBytes  int64
}

func NewHandler(storage *Storage, publisher *Publisher, maxMB int64) *Handler {
	return &Handler{
		storage:   storage,
		publisher: publisher,
		maxBytes:  maxMB * 1024 * 1024,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func validateTarGz(r io.Reader) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("invalid gzip: %w", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	if _, err := tr.Next(); err != nil {
		return fmt.Errorf("invalid tar: %w", err)
	}
	return nil
}

func (h *Handler) handleSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes)
	if err := r.ParseMultipartForm(2 << 20); err != nil { // 2MB memory limit, rest to disk
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file too large"})
		return
	}

	language := r.FormValue("language")
	if !allowedLanguages[language] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported language"})
		return
	}

	teamName := r.FormValue("team_name")
	if teamName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "team_name required"})
		return
	}

	file, header, err := r.FormFile("source")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing source file"})
		return
	}
	defer file.Close()

	if err := validateTarGz(file); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid archive"})
		return
	}

	// Rewind the file back to the beginning before uploading
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		log.Printf("seek upload: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	submissionID := uuid.New().String()
	artifactPath := fmt.Sprintf("submissions/%s.tar.gz", submissionID)

	if err := h.storage.Upload(
		r.Context(),
		artifactPath,
		file,
		header.Size,
	); err != nil {
		log.Printf("upload to seaweedfs: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	event := SubmissionCreatedEvent{
		SubmissionID: submissionID,
		Language:     language,
		TeamName:     teamName,
		ArtifactPath: artifactPath,
	}
	if err := h.publisher.PublishSubmissionCreated(r.Context(), event); err != nil {
		log.Printf("publish event: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	log.Printf("submission created: id=%s lang=%s team=%s", submissionID, language, teamName)
	writeJSON(w, http.StatusAccepted, map[string]string{"submission_id": submissionID})
}
