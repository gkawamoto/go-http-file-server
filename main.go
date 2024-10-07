package main

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/dustin/go-humanize"
)

//go:embed index.html
var indexTemplate string

//go:embed static
var static embed.FS

type FileType string

const (
	FolderFileType     FileType = "folder"
	FolderBackFileType FileType = "folder-back"
	MediaFileType      FileType = "media"
	FileFileType       FileType = "file"
)

type file struct {
	Name         string   `json:"name"`
	URL          string   `json:"url"`
	LastModified string   `json:"last_modified"`
	Size         string   `json:"size"`
	Type         FileType `json:"type"`
}

func main() {
	port := flag.Int("port", 8080, "Port to listen to")
	addr := flag.String("addr", "0.0.0.0", "Address to listen to")

	directoryPath := flag.String("dir", ".", "Directory to serve")

	flag.Parse()

	tplt, err := template.New("").Parse(indexTemplate)
	if err != nil {
		log.Panic(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/files/", http.StatusTemporaryRedirect)
	})
	http.HandleFunc("/files/", listDir(*directoryPath, tplt))
	http.Handle("/static/", http.FileServerFS(static))

	log.Printf("Server started at http://%s:%d\n", *addr, *port)

	if err := http.ListenAndServe(fmt.Sprintf("%s:%d", *addr, *port), nil); err != nil {
		log.Panicf("Error starting server: %s\n", err)
	}
}

func sanitizePath(directoryPath, path string) string {
	parts := []string{}
	for _, p := range strings.Split(path, "/")[2:] {
		if p == ".." {
			continue
		}

		parts = append(parts, p)
	}

	return filepath.Join(append([]string{directoryPath}, parts...)...)
}

func isMediaFile(entry os.DirEntry) bool {
	for _, ext := range []string{".mp4", ".mkv", ".avi"} {
		if strings.HasSuffix(entry.Name(), ext) {
			return true
		}
	}

	return false
}

func toFileList(entry os.DirEntry) (file, error) {
	name := entry.Name()
	info, err := entry.Info()
	if err != nil {
		return file{}, err
	}

	if entry.IsDir() {
		name = name + "/"
		return file{
			Name:         name,
			URL:          name,
			LastModified: humanize.Time(info.ModTime()),
			Size:         humanize.BigBytes(big.NewInt(info.Size())),
			Type:         FolderFileType,
		}, nil
	}
	if isMediaFile(entry) {
		return file{
			Name:         name,
			URL:          url.PathEscape(name),
			LastModified: humanize.Time(info.ModTime()),
			Size:         humanize.BigBytes(big.NewInt(info.Size())),
			Type:         MediaFileType,
		}, nil
	}

	return file{
		Name:         name,
		URL:          url.PathEscape(name),
		LastModified: humanize.Time(info.ModTime()),
		Size:         humanize.BigBytes(big.NewInt(info.Size())),
		Type:         FileFileType,
	}, nil
}

func listDirContents(path string) ([]file, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	files := []file{}

	for _, entry := range entries {
		f, err := toFileList(entry)
		if err != nil {
			return nil, err
		}

		files = append(files, f)
	}

	return files, nil
}

func listDir(directoryPath string, tplt *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := sanitizePath(directoryPath, r.URL.Path)
		stat, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, fmt.Sprintf("%s not found", path), http.StatusNotFound)
			return
		}

		if !stat.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer f.Close()

			http.ServeContent(w, r, path, stat.ModTime(), f)
			return
		}

		if !strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, r.URL.Path+"/", http.StatusTemporaryRedirect)
			return
		}

		entries, err := listDirContents(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if r.URL.Path != "/files/" {
			entries = append([]file{{
				Name: "..",
				URL:  "..",
				Type: FolderBackFileType,
			}}, entries...)
		}

		data, err := json.Marshal(entries)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := tplt.Execute(w, map[string]any{
			"entries": strings.ReplaceAll(string(data), `"`, `\"`),
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
