package main

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/samber/lo"
)

const publicBaseUrl = "/public/"
const imgBaseUrl = "/img/"

type Index struct {
	Title  string
	Photos []string
}

type FileHolder struct {
	Mu    sync.RWMutex
	Files []os.DirEntry
}

// helper method to set files. Handles locking
func (f *FileHolder) Set(files []os.DirEntry) {
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
		Files: fileEntries,
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
				log.Println("watcherEvent: ", event)
				hasNewEvent = true
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
					fileHolder.Set(fileEntries)
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
	imgServer := http.FileServer(http.Dir(homePath))
	http.Handle(imgBaseUrl, http.StripPrefix(imgBaseUrl, imgServer))

	publicServer := http.FileServer(http.Dir("./public"))
	http.Handle(publicBaseUrl, http.StripPrefix(publicBaseUrl, publicServer))

	// --- Routes ---
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// FIXME is it bad to aquire a read lock all the time here?
		// Maybe better to just have eventual consistency
		// worst that could happen is the page loads with some dead image links, solved by refresh
		fileHolder.Mu.RLock()
		defer fileHolder.Mu.RUnlock()

		f := namesFromEntries(fileHolder.Files)
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
	log.Printf("starting server on port %s", serverPort)
	if err := http.ListenAndServe(serverPort, logRequest(http.DefaultServeMux)); err != nil {
		log.Fatal(err)
	}
}

func loadHomePath(homePath string) ([]os.DirEntry, error) {
	log.Println("Loading homePath from: ", homePath)
	files, err := os.ReadDir(homePath)
	if err != nil {
		return nil, err
	}
	files = lo.Filter(files, isFiletypeAllowed)
	return files, nil
}

func replaceWindowsPathSeparator(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}

func isFiletypeAllowed(file os.DirEntry, index int) bool {
	whitelist := []string{"png", "jpeg", "jpg", "svg", "gif"}
	f := file.Name()
	t := f[strings.LastIndex(f, ".")+1:]

	return stringInSlice(strings.ToLower(t), whitelist)
}

func logRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

func namesFromEntries(paths []os.DirEntry) []string {
	res := make([]string, len(paths))
	for i, path := range paths {
		res[i] = path.Name()
	}
	return res
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
