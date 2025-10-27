package main

import (
	"album/internal/application"
	"album/internal/handler"
	"album/internal/images"

	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/samber/lo"
)

func main() {
	// --- Setup ---
	if len(os.Args) != 2 {
		fmt.Println("USAGE: ./fotodeck <CONFIG PATH>")
		os.Exit(1)
	}

	configPath := os.Args[1]
	conf, err := application.LoadConfig(configPath)
	if err != nil {
		slog.Error("failed to load application config", "path", configPath, "error", err.Error())
		os.Exit(1)
	}
	err = application.ValidateHomePath(conf)
	if err != nil {
		slog.Error("failed to validate home path", "path", conf.Home.Path, "error", err.Error())
		os.Exit(1)
	}

	// --- Load files ---
	loader := images.Loader{
		OptimisedExtension: conf.ImageResizing.ResizedFileExtension,
		PreviewExtension:   conf.ImageResizing.PreviewFileExtension,
		MaxOptimisedDimensions: images.Dimensions{
			Width:  conf.ImageResizing.ResizedWidth,
			Height: conf.ImageResizing.ResizedHeight,
		},
		MaxPreviewDimensions: images.Dimensions{
			Width:  conf.ImageResizing.PreviewWidth,
			Height: conf.ImageResizing.PreviewHeight,
		},
	}
	var fileEntries map[string]images.ImageFile
	fileEntries, fileLoadErr := loader.LoadOriginals(conf.Home.Path)
	if fileLoadErr != nil {
		log.Fatal(fileLoadErr)
	}
	fileLoadErr = loader.OptimiseImages(&fileEntries)
	if fileLoadErr != nil {
		slog.Error("failed to optimimise images: ", "error", fileLoadErr)
	}
	fileHolder := handler.FileHolder{
		Files: lo.Keys(fileEntries),
	}
	log.Printf("Found %d photos in %s", len(fileHolder.Files), conf.Home.Path)

	// --- Watch for file changes ---
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("failed to initialise file watcher. File watch will be disabled", "error", err)
	}
	if watcher != nil {
		defer watcher.Close()
	}

	throttle := time.NewTicker(time.Duration(conf.Home.MinRefreshInterval) * time.Second)
	defer throttle.Stop()

	go fileWatchFn(watcher, &loader, &fileHolder, throttle, conf.Home.Path)

	err = watcher.Add(conf.Home.Path)
	if err != nil {
		slog.Error("failed to add home path to file watcher. File watch will be disabled", "error", err)
	}

	// --- Static file servers ---
	publicServer := http.FileServer(http.Dir("./web/static"))
	http.Handle("/public/", http.StripPrefix("/public/", publicServer))

	// --- Routes ---
	rootHandler := handler.RootHandler{
		FileHolder: &fileHolder,
	}

	imageHandler := handler.ImageHandler{
		FileEntries: fileEntries,
	}

	http.HandleFunc("/img/preview/{id}", imageHandler.Previews)

	http.HandleFunc("/img/{id}", imageHandler.Images)

	http.HandleFunc("/", rootHandler.Index)

	// --- Run ---
	server := &http.Server{
		Addr:              conf.Server.ListenAddr,
		ReadTimeout:       5 * time.Second,
		ReadHeaderTimeout: 3 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	go func() {
		slog.Info("starting server", "addr", conf.Server.ListenAddr)
		// #nosec G114 -- headers are set above
		if err := http.ListenAndServe(conf.Server.ListenAddr, logRequest(http.DefaultServeMux)); !errors.Is(err, http.ErrServerClosed) {
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

	if conf.ImageResizing.CleanupOnShutdown {
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

func logRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("HttpServer", "remoteAddr", r.RemoteAddr, "method", r.Method, "url", r.URL)
		handler.ServeHTTP(w, r)
	})
}

func fileWatchFn(watcher *fsnotify.Watcher, loader *images.Loader, fileHolder *handler.FileHolder, throttle *time.Ticker, homePath string) {
	var hasNewEvent bool

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// prevent circular update loop
			if !loader.IsResizedImage(event.Name) {
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
				fileEntries, fileLoadErr := loader.Reload(homePath)
				if fileLoadErr != nil {
					slog.Error("failed to reload homePath", "path", fileLoadErr)
				}
				fileHolder.Set(lo.Keys(fileEntries))
				slog.Info("watcherEvent: homePath refresh completed")
				hasNewEvent = false
			}
		}
	}
}
