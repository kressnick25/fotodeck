package handler

import (
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"text/template"
	"time"
)

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

type IndexTemplate struct {
	Title  string
	Photos []string
}

type RootHandler struct {
	FileHolder *FileHolder
}

func (rh *RootHandler) Index(w http.ResponseWriter, r *http.Request) {
	// FIXME is it bad to aquire a read lock all the time here?
	// Maybe better to just have eventual consistency
	// worst that could happen is the page loads with some dead image links, solved by refresh
	rh.FileHolder.Mu.RLock()
	defer rh.FileHolder.Mu.RUnlock()

	f := rh.FileHolder.Files
	rand.NewSource(time.Now().UnixNano())
	// shuffle the files slice
	for i := range f {
		j := rand.Intn(i + 1) // #nosec G404 -- secure random not required
		f[i], f[j] = f[j], f[i]
	}

	data := IndexTemplate{
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
}
