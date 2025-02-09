package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"album/internal/resize"

	"github.com/fsnotify/fsnotify"
	"github.com/samber/lo"
)

const doResizeCleanup = false

const publicBaseUrl = "/public/"
const imgBaseUrl = "/img/"

const optimisedMaxHeight = 2000
const optimisedMaxWidth = 2000
const optimsedExtension = "optimised"

const previewMaxHeight = 600
const previewMaxWidth = 600
const previewExtension = "preview"

type Index struct {
	Title  string
	Photos []string
}

type FilePath struct {
	Optimised string
	Preview   string
	Original  string
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
	fileEntries, fileLoadErr := loadHomePath(homePath)
	if fileLoadErr != nil {
		log.Fatal(fileLoadErr)
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
				if !strings.Contains(event.Name, optimsedExtension) && !strings.Contains(event.Name, previewExtension) {
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
					fileEntries, fileLoadErr = loadHomePath(homePath)
					if fileLoadErr != nil {
						log.Printf("error: failed to reload homePath: %v\n", fileLoadErr)
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
	publicServer := http.FileServer(http.Dir("./public"))
	http.Handle(publicBaseUrl, http.StripPrefix(publicBaseUrl, publicServer))

	// --- Routes ---
	http.HandleFunc("/img/preview/{id}", func(w http.ResponseWriter, r *http.Request) {
		requestFile := r.PathValue("id")
		entry, ok := fileEntries[requestFile]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		http.ServeFile(w, r, entry.Preview)
	})

	http.HandleFunc("/img/{id}", func(w http.ResponseWriter, r *http.Request) {
		requestFile := r.PathValue("id")
		entry, ok := fileEntries[requestFile]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var path string
		if entry.Optimised != "" {
			path = entry.Optimised
		} else {
			path = entry.Original
		}

		http.ServeFile(w, r, path)
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
		t, _ := template.ParseFiles("template/index.html")
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
		cleanupResized(homePath)
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP shutdown error: %v", err)
	}
	log.Println("Graceful shutdown complete.")
}

func loadHomePath(homePath string) (map[string]FilePath, error) {
	log.Println("Loading homePath from: ", homePath)
	fileMap := make(map[string]FilePath)
	err := filepath.WalkDir(homePath, func(path string, f os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !f.Type().IsRegular() {
			return nil
		}
		if strings.Contains(f.Name(), optimsedExtension) || strings.Contains(f.Name(), previewExtension) {
			// TODO debug log
			log.Println("import: skipping already optimised file: ", f.Name())
			return nil
		}
		if !isFiletypeAllowed(f.Name()) {
			// TODO debug log
			log.Println("import: skipping non-image file: ", f.Name())
			return nil
		}

		existingPath, ok := fileMap[f.Name()]
		if ok {
			log.Printf("warning: duplicate filename entry found at %s. %s will be used\n", existingPath, path)
		}

		optimisedPath := resizeImage(path, optimsedExtension, optimisedMaxWidth, optimisedMaxHeight)
		previewPath := resizeImage(path, previewExtension, previewMaxWidth, previewMaxHeight)
		fileMap[f.Name()] = FilePath{
			Optimised: optimisedPath,
			Preview:   previewPath,
			Original:  path,
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return fileMap, nil
}

func cleanupResized(homePath string) error {
	err := filepath.WalkDir(homePath, func(path string, f os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(f.Name(), optimsedExtension) || strings.Contains(f.Name(), previewExtension) {
			// TODO debug log
			log.Println("removing optimised file: ", path)
			os.Remove(path)
		}
		return nil
	})
	return err
}

func resizeImage(inputPath string, extension string, maxWidth int, maxHeight int) string {
	outputPath := getOptimisedFilePath(inputPath, extension)
	if _, err := os.Stat(outputPath); err == nil {
		// TODO debug log
		log.Println("resizeImage: Resized image already exists, skipping: ", outputPath)
		return outputPath
	}

	log.Printf("resizeImage: generating %s image: %s\n", extension, outputPath)
	image, err := resize.Open(inputPath)
	if err != nil {
		log.Println("resizeImage: Error opening image to resize: ", err)
		return inputPath
	}

	opts := resize.Options{
		MaxWidth:  maxWidth,
		MaxHeight: maxHeight,
	}
	image = resize.Resize(image, opts)

	err = resize.Save(image, outputPath)
	if err != nil {
		log.Println("resizeImage: error saving image to resize: ", err)
		return inputPath
	}

	return outputPath
}

func getOptimisedFilePath(inputPath string, extension string) string {
	paths := strings.Split(inputPath, ".")

	if paths[len(paths)-2] == optimsedExtension {
		// already an optimised file
		return inputPath
	}

	// transform 'image.jpg' -> 'image.optimised.jpg'
	tmp := paths[len(paths)-1]
	paths[len(paths)-1] = extension
	paths = append(paths, tmp)

	return strings.Join(paths, ".")
}

func replaceWindowsPathSeparator(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}

func isFiletypeAllowed(fileName string) bool {
	whitelist := []string{"png", "jpeg", "jpg", "svg", "gif"}
	_type := fileName[strings.LastIndex(fileName, ".")+1:]

	return stringInSlice(strings.ToLower(_type), whitelist)
}

func logRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
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
