package files

import (
	"encoding/json"
	"errors"
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

	_ "embed"
)

type fileType string

const (
	folderFileType     fileType = "folder"
	folderBackFileType fileType = "folder-back"
	mediaFileType      fileType = "media"
	fileFileType       fileType = "file"
)

type file struct {
	Name         string   `json:"name"`
	URL          string   `json:"url"`
	LastModified string   `json:"last_modified"`
	Size         string   `json:"size"`
	Type         fileType `json:"type"`
}

//go:embed index.html
var indexFile string

var tplt *template.Template

func init() {
	var err error
	tplt, err = template.New("").Parse(indexFile)
	if err != nil {
		log.Panic(err)
	}
}

type handler struct {
	directoryPath string
}

func NewHandler(directoryPath string) http.Handler {
	return &handler{
		directoryPath: directoryPath,
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
			Type:         folderFileType,
		}, nil
	}
	if isMediaFile(entry) {
		return file{
			Name:         name,
			URL:          url.PathEscape(name),
			LastModified: humanize.Time(info.ModTime()),
			Size:         humanize.BigBytes(big.NewInt(info.Size())),
			Type:         mediaFileType,
		}, nil
	}

	return file{
		Name:         name,
		URL:          url.PathEscape(name),
		LastModified: humanize.Time(info.ModTime()),
		Size:         humanize.BigBytes(big.NewInt(info.Size())),
		Type:         fileFileType,
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

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := sanitizePath(h.directoryPath, r.URL.Path)
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
			Type: folderBackFileType,
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
