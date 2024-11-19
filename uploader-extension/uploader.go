package uploaderextension

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"go.opentelemetry.io/collector/component"
)

type uploaderExtension struct {
	config *Config
	server *http.Server
}

func newUploaderExtension(cfg *Config) *uploaderExtension {
	return &uploaderExtension{
		config: cfg,
	}
}

// Start starts the extension.
func (ue *uploaderExtension) Start(ctx context.Context, host component.Host) error {
	println("Starting the extension")
	println("Endpoint: ", ue.config.Endpoint)
	ue.server = &http.Server{
		Addr:    ue.config.Endpoint,
		Handler: http.HandlerFunc(ue.handleUpload),
	}
	go func() {
		if err := ue.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting custom extension server: %v", err)
		}
	}()
	// Add your startup logic here.
	return nil
}

// Shutdown stops the extension.
func (ue *uploaderExtension) Shutdown(ctx context.Context) error {
	// Add your shutdown logic here.
	return nil
}

// handleUpload processes incoming file uploads.
func (ue *uploaderExtension) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST requests are accepted", http.StatusMethodNotAllowed)
		return
	}

	// Parse the form with a max upload size of 10MB
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	// Retrieve the uploaded file from the form-data
	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to retrieve file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Define the destination file path
	destPath := filepath.Join(ue.config.StorageDirectory, handler.Filename)
	log.Default().Printf("Saving file to %s", destPath)

	// Create the destination file
	outFile, err := os.Create(destPath)
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}
	defer outFile.Close()

	// Copy the file contents to the destination file
	if _, err = io.Copy(outFile, file); err != nil {
		http.Error(w, "Failed to write file", http.StatusInternalServerError)
		return
	}

	// Respond with success message
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("File saved successfully at %s", destPath)))
}
