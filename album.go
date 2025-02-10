package main

import (
	"album/internal/images"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/samber/lo"
)

const doResizeCleanup = false

const optimisedMaxHeight = 2000
const optimisedMaxWidth = 2000
const optimisedExtension = "optimised"

const previewMaxHeight = 600
const previewMaxWidth = 600
const previewExtension = "preview"

type Index struct {
	Title  string
	Photos []string
}

type FileHolder struct {
	Mu    sync.RWMutex
	Files []string
}

// helper method to set files. Handles locking
func (f *FileHolder) Set(files []string) {
	f.Mu.Lock()
	defer f.Mu.Unlock()

	f.Files = files
}

func main() {
	// --- Setup ---
	if len(os.Args) != 3 {
		fmt.Println("USAGE: ./album <PORT> <PHOTOS_PATH>")
		os.Exit(1)
	}
	port := os.Args[1]
	homePath := os.Args[2]
	homePath = replaceWindowsPathSeparator(homePath)
	validatePath(homePath)

	// --- Load files ---
	maxPreviewDimensions := images.Dimensions{
		Width:  previewMaxWidth,
		Height: previewMaxHeight,
	}
	maxOptimisedDimensions := images.Dimensions{
		Width:  optimisedMaxWidth,
		Height: optimisedMaxHeight,
	}
	loader := images.Loader{
		OptimisedExtension: optimisedExtension,
		PreviewExtension:   previewExtension,
	}
	var fileEntries map[string]images.ImageFile
	fileEntries, fileLoadErr := loader.LoadOriginals(homePath)
	if fileLoadErr != nil {
		log.Fatal(fileLoadErr)
	}
	fileLoadErr = loader.OptimiseImages(&fileEntries, maxOptimisedDimensions, maxPreviewDimensions)
	if fileLoadErr != nil {
		slog.Error("failed to optimimise images: ", "err", fileLoadErr)
	}
	fileHolder := FileHolder{
		Files: lo.Keys(fileEntries),
	}
	log.Printf("Found %d photos in %s", len(fileHolder.Files), homePath)

	// --- Watch for file changes ---
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("failed to initialise file watcher. File watch will be disabled", "error", err)
	}
	if watcher != nil {
		defer watcher.Close()
	}

	// TODO make this time configurable
	throttle := time.NewTicker(1 * time.Second)
	defer throttle.Stop()

	go func() {
		var hasNewEvent bool

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// prevent circular update loop
				if !images.IsResizedImage(event.Name) {
					slog.Info("watcherEvent", "event", event)
					hasNewEvent = true
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Error("watcherError: ", "err", err)
			case <-throttle.C:
				if hasNewEvent {
					fileEntries, fileLoadErr = loader.LoadOriginals(homePath)
					if fileLoadErr != nil {
						slog.Error("failed to reload homePath", "path", fileLoadErr)
					}
					fileLoadErr := loader.OptimiseImages(&fileEntries, maxOptimisedDimensions, maxPreviewDimensions)
					if fileLoadErr != nil {
						slog.Error("failed to re-optimise images", "error", fileLoadErr)
					}
					fileHolder.Set(lo.Keys(fileEntries))
					slog.Info("watcherEvent: homePath refresh completed")
					hasNewEvent = false
				}
			}
		}
	}()

	err = watcher.Add(homePath)
	if err != nil {
		slog.Error("failed to add home path to file watcher. File watch will be disabled", "error", err)
	}

	// --- Static file servers ---
	publicServer := http.FileServer(http.Dir("./web/static"))
	http.Handle("/public/", http.StripPrefix("/public/", publicServer))

	// --- Routes ---
	http.HandleFunc("/img/preview/{id}", func(w http.ResponseWriter, r *http.Request) {
		requestFile := r.PathValue("id")
		entry, ok := fileEntries[requestFile]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		slog.Debug("", "requestFile", "/img/preview/"+requestFile, "responseFile", entry.GetPreview())

		http.ServeFile(w, r, entry.GetPreview())
	})

	http.HandleFunc("/img/{id}", func(w http.ResponseWriter, r *http.Request) {
		requestFile := r.PathValue("id")
		entry, ok := fileEntries[requestFile]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		slog.Debug("", "requestFile", "/img/"+requestFile, "responseFile", entry.GetFullSize())

		http.ServeFile(w, r, entry.GetFullSize())
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// FIXME is it bad to aquire a read lock all the time here?
		// Maybe better to just have eventual consistency
		// worst that could happen is the page loads with some dead image links, solved by refresh
		fileHolder.Mu.RLock()
		defer fileHolder.Mu.RUnlock()

		f := fileHolder.Files
		rand.NewSource(time.Now().UnixNano())
		// shuffle the files slice
		for i := range f {
			j := rand.Intn(i + 1) // #nosec G404 -- secure random not required
			f[i], f[j] = f[j], f[i]
		}

		data := Index{
			Title:  "My Album",
			Photos: f,
		}

		templateFile := "web/template/index.html"
		t, err := template.ParseFiles(templateFile)
		if err != nil {
			slog.Error("Failed to parse template", "template", templateFile, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		err = t.Execute(w, &data)
		if err != nil {
			slog.Error("Failed to execute template", "template", templateFile, "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	})

	// --- Run ---
	serverPort := fmt.Sprintf(":%s", port)
	server := &http.Server{
		Addr:              serverPort,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	go func() {
		slog.Info("starting server", "port", port)
		// #nosec G114 -- headers are set above
		if err := http.ListenAndServe(serverPort, logRequest(http.DefaultServeMux)); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		slog.Info("Stopped serving new connections.")
	}()

	// --- Graceful shutdown ---
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if doResizeCleanup {
		err = watcher.Close()
		if err != nil {
			slog.Error("Failed to close watcher", "error", err)
		}
		for _, v := range fileEntries {
			err := v.Cleanup()
			if err != nil {
				slog.Error("error cleaning up file", "file", v.Name(), "error", err)
			}
		}
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP shutdown error", "error", err)
		os.Exit(1)
	}
	slog.Info("Graceful shutdown complete.")
}

func replaceWindowsPathSeparator(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}

func logRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("HttpServer", "remoteAddr", r.RemoteAddr, "method", r.Method, "url", r.URL)
		handler.ServeHTTP(w, r)
	})
}

func validatePath(homePath string) {
	s, err := os.Stat(homePath)
	if errors.Is(err, os.ErrNotExist) {
		slog.Error("path does not exist", "path", homePath)
		os.Exit(1)
	}
	if !s.IsDir() {
		slog.Error("path is not a directory", "path", homePath)
		os.Exit(1)
	}
}
