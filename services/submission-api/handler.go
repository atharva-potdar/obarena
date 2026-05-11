package main

import (
	"archive/tar"
	"bytes"
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

var languageFilename = map[string]string{
	"cpp":  "main.cpp",
	"rust": "main.rs",
	"go":   "main.go",
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

func normalizeToTarGz(data []byte, language string) ([]byte, error) {
	// Already a tar.gz
	if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
		return data, nil
	}

	// Raw source file — wrap it in a tar.gz
	filename := languageFilename[language]

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	if err := tw.WriteHeader(&tar.Header{
		Name: filename,
		Mode: 0o644,
		Size: int64(len(data)),
	}); err != nil {
		return nil, fmt.Errorf("write tar header: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return nil, fmt.Errorf("write tar body: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}
	return buf.Bytes(), nil
}

func (h *Handler) handleSubmit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes)
	if err := r.ParseMultipartForm(h.maxBytes); err != nil {
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

	file, _, err := r.FormFile("source")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing source file"})
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		log.Printf("read upload: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	normalized, err := normalizeToTarGz(raw, language)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid archive"})
		return
	}

	submissionID := uuid.New().String()
	artifactPath := fmt.Sprintf("submissions/%s.tar.gz", submissionID)

	if err := h.storage.Upload(
		r.Context(),
		artifactPath,
		bytes.NewReader(normalized),
		int64(len(normalized)),
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
