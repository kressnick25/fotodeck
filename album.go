package main

import (
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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
	"github.com/samber/lo"
)

type (
	config struct {
		Home          home
		Server        server
		ImageResizing imageResizing
	}

	imageResizing struct {
		Async                bool
		CleanupOnShutdown    bool
		Enabled              bool
		PreviewWidth         int
		PreviewHeight        int
		ResizedWidth         int
		ResizedHeight        int
		ResizedFileExtension string
		PreviewFileExtension string
	}

	server struct {
		ListenAddr string
	}

	home struct {
		Path               string
		MinRefreshInterval int
	}
)

func main() {
	// --- Setup ---
	if len(os.Args) != 2 {
		fmt.Println("USAGE: ./album <CONFIG PATH>")
		os.Exit(1)
	}
	configPath := os.Args[1]
	configPath = replaceWindowsPathSeparator(configPath)
	filepath.Clean(configPath)
	validatePath(configPath)

	configFileBytes, err := os.ReadFile(configPath)
	if err != nil {
		panic(err)
	}
	configFileString := string(configFileBytes)

	var conf config
	_, err = toml.Decode(configFileString, &conf)
	if err != nil {
		panic(err)
	}
	slog.Info("Loaded config file", "path", configPath)

	conf.Home.Path = replaceWindowsPathSeparator(conf.Home.Path)
	conf.Home.Path = filepath.Clean(conf.Home.Path)
	validateHomePath(conf.Home.Path)

	// --- Load files ---
	maxPreviewDimensions := images.Dimensions{
		Width:  conf.ImageResizing.PreviewWidth,
		Height: conf.ImageResizing.PreviewHeight,
	}
	maxOptimisedDimensions := images.Dimensions{
		Width:  conf.ImageResizing.ResizedWidth,
		Height: conf.ImageResizing.ResizedHeight,
	}
	loader := images.Loader{
		OptimisedExtension: conf.ImageResizing.ResizedFileExtension,
		PreviewExtension:   conf.ImageResizing.PreviewFileExtension,
	}
	var fileEntries map[string]images.ImageFile
	fileEntries, fileLoadErr := loader.LoadOriginals(conf.Home.Path)
	if fileLoadErr != nil {
		log.Fatal(fileLoadErr)
	}
	fileLoadErr = loader.OptimiseImages(&fileEntries, maxOptimisedDimensions, maxPreviewDimensions)
	if fileLoadErr != nil {
		slog.Error("failed to optimimise images: ", "err", fileLoadErr)
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

	go func() {
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
					fileEntries, fileLoadErr = loader.LoadOriginals(conf.Home.Path)
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

func replaceWindowsPathSeparator(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}

func logRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("HttpServer", "remoteAddr", r.RemoteAddr, "method", r.Method, "url", r.URL)
		handler.ServeHTTP(w, r)
	})
}

func validateHomePath(homePath string) {
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

func validatePath(homePath string) {
	_, err := os.Stat(homePath)
	if errors.Is(err, os.ErrNotExist) {
		slog.Error("path does not exist", "path", homePath)
		os.Exit(1)
	}
}
