package main

import (
	"album/internal/images"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
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

const publicBaseUrl = "/public/"
const imgBaseUrl = "/img/"

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
		log.Printf("error: failed to optimise images: %v\n", fileLoadErr)
	}
	fileHolder := FileHolder{
		Files: lo.Keys(fileEntries),
	}
	log.Printf("Found %d photos in %s", len(fileHolder.Files), homePath)

	// --- Watch for file changes ---
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		// TODO log err and disable file watching
		log.Fatal(err)
	}
	defer watcher.Close()

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
					log.Println("watcherEvent: ", event)
					hasNewEvent = true
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("watcherError: ", err)
			case <-throttle.C:
				if hasNewEvent {
					fileEntries, fileLoadErr = loader.LoadOriginals(homePath)
					if fileLoadErr != nil {
						log.Printf("error: failed to reload homePath: %v\n", fileLoadErr)
					}
					fileLoadErr := loader.OptimiseImages(&fileEntries, maxOptimisedDimensions, maxPreviewDimensions)
					if fileLoadErr != nil {
						log.Printf("error: failed to re-optimise images: %v\n", fileLoadErr)
					}
					fileHolder.Set(lo.Keys(fileEntries))
					log.Println("watcherEvent: homePath refresh completed")
					hasNewEvent = false
				}
			}
		}
	}()

	err = watcher.Add(homePath)
	if err != nil {
		// TODO log err and disable file watching
		log.Fatal(err)
	}

	// --- Static file servers ---
	publicServer := http.FileServer(http.Dir("./web/static"))
	http.Handle(publicBaseUrl, http.StripPrefix(publicBaseUrl, publicServer))

	// --- Routes ---
	http.HandleFunc("/img/preview/{id}", func(w http.ResponseWriter, r *http.Request) {
		requestFile := r.PathValue("id")
		entry, ok := fileEntries[requestFile]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// TODO debug log
		fmt.Printf("requested: /img/preview/%s, served: %s\n", requestFile, entry.GetPreview())

		http.ServeFile(w, r, entry.GetPreview())
	})

	http.HandleFunc("/img/{id}", func(w http.ResponseWriter, r *http.Request) {
		requestFile := r.PathValue("id")
		entry, ok := fileEntries[requestFile]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// TODO debug log
		fmt.Printf("requested: /img/%s, served: %s\n", requestFile, entry.GetFullSize())

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
			j := rand.Intn(i + 1)
			f[i], f[j] = f[j], f[i]
		}

		data := Index{
			Title:  "My Album",
			Photos: f,
		}
		t, _ := template.ParseFiles("web/template/index.html")
		t.Execute(w, &data)
	})

	// --- Run ---
	serverPort := fmt.Sprintf(":%s", port)
	server := &http.Server{
		Addr: serverPort,
	}

	go func() {
		log.Printf("starting server on port %s", serverPort)
		if err := http.ListenAndServe(serverPort, logRequest(http.DefaultServeMux)); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		log.Println("Stopped serving new connections.")
	}()

	// --- Graceful shutdown ---
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if doResizeCleanup {
		watcher.Close()
		for _, v := range fileEntries {
			err := v.Cleanup()
			if err != nil {
				// TODO error log
				log.Println("error: cleaning up file :", v.Name(), err)
			}
		}
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP shutdown error: %v", err)
	}
	log.Println("Graceful shutdown complete.")
}

func replaceWindowsPathSeparator(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}

func logRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

func validatePath(homePath string) {
	s, err := os.Stat(homePath)
	if errors.Is(err, os.ErrNotExist) {
		fmt.Printf("error: path does not exist: '%s'\n", homePath)
		os.Exit(1)
	}
	if !s.IsDir() {
		fmt.Printf("error: path is not a directory: '%s'\n", homePath)
		os.Exit(1)
	}
}
