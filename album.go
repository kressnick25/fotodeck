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
	"time"
)

const publicBaseUrl = "/public/"
const imgBaseUrl = "/img/"

type Index struct {
	Title  string
	Photos []string
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println("USAGE: ./album <PORT> <PHOTOS_PATH>")
		os.Exit(1)
	}
	port := os.Args[1]
	homePath := os.Args[2]
	validatePath(homePath)

	files, err := os.ReadDir(homePath)
	if err != nil {
		panic(err)
	}

	for i, file := range files {
		if !isFiletypeAllowed(file) {
			log.Printf("warning: removing illegal file from index %s", file.Name())
			// remove
			files[i] = files[len(files)-1]
			files = files[:len(files)-1]
		}
	}

	log.Printf("Found %d photos in %s", len(files), homePath)

	imgServer := http.FileServer(http.Dir(homePath))
	http.Handle(imgBaseUrl, http.StripPrefix(imgBaseUrl, imgServer))

	publicServer := http.FileServer(http.Dir("./public"))
	http.Handle(publicBaseUrl, http.StripPrefix(publicBaseUrl, publicServer))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f := namesFromEntries(files)
		rand.NewSource(time.Now().UnixNano())
		// shuffe the files slice
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

	serverPort := fmt.Sprintf(":%s", port)
	log.Printf("starting server on port %s", serverPort)
	if err := http.ListenAndServe(serverPort, logRequest(http.DefaultServeMux)); err != nil {
		log.Fatal(err)
	}
}

func isFiletypeAllowed(file os.DirEntry) bool {
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
