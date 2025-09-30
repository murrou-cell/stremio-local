package main

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type MediaItem struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	RelPath   string   `json:"relPath"`   // relative to mediaDir
	Subtitles []string `json:"subtitles"` // also relative paths
}

var mediaLibrary = []MediaItem{}
var mediaDir = "media"

func main() {
	// Scan media dir on startup
	scanMediaDir(mediaDir)

	http.HandleFunc("/manifest.json", manifestHandler)
	http.HandleFunc("/catalog/movie/", catalogHandler)
	http.HandleFunc("/stream/movie/", streamHandler)
	http.HandleFunc("/subtitles/movie/", subtitlesHandler)
	http.HandleFunc("/meta/movie/", metaHandler)

	// Serve actual files under /files/
	fs := http.FileServer(http.Dir(mediaDir))
	http.Handle("/files/", http.StripPrefix("/files/", fs))

	log.Println("Local Media Addon running on http://localhost:8081/manifest.json")
	log.Fatal(http.ListenAndServe(":8081", nil))
}

// --- Handlers ---

func metaHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/meta/movie/")
	path = strings.TrimSuffix(path, ".json")
	id := path

	for _, item := range mediaLibrary {
		if item.ID == id {
			meta := map[string]interface{}{
				"id":          item.ID,
				"type":        "movie",
				"name":        item.Title,
				"poster":      "https://via.placeholder.com/200x300?text=" + strings.ReplaceAll(item.Title, " ", "+"),
				"background":  "https://via.placeholder.com/600x400?text=" + strings.ReplaceAll(item.Title, " ", "+"),
				"description": "Local file: " + item.Title,
				"genres":      []string{"Local"},
				"year":        2023,
			}
			writeJSON(w, map[string]interface{}{"meta": meta})
			return
		}
	}

	http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
}

func manifestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	catalogs := []map[string]interface{}{}

	for catID := range catalogMap {
		catalogs = append(catalogs, map[string]interface{}{
			"type": "movie",
			"id":   catID,
			"name": catID,
			"extra": []map[string]interface{}{
				{"name": "search", "isRequired": false},
			},
		})
	}

	manifest := map[string]interface{}{
		"id":        "stremio-local",
		"version":   "1.0.0",
		"name":      "Local Media",
		"resources": []string{"catalog", "meta", "stream", "subtitles"},
		"types":     []string{"movie"},
		"catalogs":  catalogs,
	}

	json.NewEncoder(w).Encode(manifest)
}

func catalogHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Catalog request:", r.URL.Path)

	// Strip prefix and suffix
	path := strings.TrimPrefix(r.URL.Path, "/catalog/movie/")
	path = strings.TrimSuffix(path, ".json")
	catalogID := path

	items, ok := catalogMap[catalogID]
	if !ok {
		log.Println("Catalog not found:", catalogID)
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"metas": []interface{}{}})
		return
	}

	metas := []map[string]interface{}{}
	for _, item := range items {
		metas = append(metas, map[string]interface{}{
			"id":     item.ID,
			"type":   "movie",
			"name":   item.Title,
			"poster": "https://via.placeholder.com/200x300?text=" + item.Title,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"metas": metas})
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/stream/movie/"), ".json")

	for _, item := range mediaLibrary {
		if item.ID == id {
			stream := map[string]interface{}{
				"url":   "http://localhost:8081/files/" + item.RelPath,
				"title": "Local File",
			}
			writeJSON(w, map[string]interface{}{"streams": []interface{}{stream}})
			return
		}
	}
	writeJSON(w, map[string]interface{}{"streams": []interface{}{}})
}

func subtitlesHandler(w http.ResponseWriter, r *http.Request) {
	// Remove prefix
	path := strings.TrimPrefix(r.URL.Path, "/subtitles/movie/")
	// Remove anything after first .json
	idx := strings.Index(path, ".json")
	if idx != -1 {
		path = path[:idx]
	}
	// Only take the first segment before any "/filename=" query
	parts := strings.Split(path, "/filename=")
	id := parts[0]

	// Normalize slashes
	id = strings.ReplaceAll(id, "\\", "/")

	// Find media item
	for _, item := range mediaLibrary {
		if item.ID == id {
			subs := []map[string]string{}
			for _, sub := range item.Subtitles {
				subs = append(subs, map[string]string{
					"id":   strings.TrimSuffix(filepath.Base(sub), filepath.Ext(sub)),
					"url":  "http://localhost:8081/files/" + sub,
					"lang": detectLang(sub),
				})
			}
			writeJSON(w, map[string]interface{}{"subtitles": subs})
			return
		}
	}

	// fallback: empty subtitles
	writeJSON(w, map[string]interface{}{"subtitles": []interface{}{}})
}

// --- Helpers ---

var catalogMap = map[string][]MediaItem{}

func scanMediaDir(dir string) {
	catalogMap = map[string][]MediaItem{}
	mediaLibrary = []MediaItem{} // flat list if needed

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		if ext != ".mp4" && ext != ".mkv" {
			return nil
		}

		base := strings.TrimSuffix(info.Name(), ext)

		// relative path from media root
		relPath, _ := filepath.Rel(dir, path)
		relPath = filepath.ToSlash(relPath)

		id := strings.TrimSuffix(relPath, ext)

		// Detect catalog: top-level folder
		parts := strings.Split(relPath, "/")
		catalogID := parts[0] // top folder name

		// Scan subtitles
		folder := filepath.Dir(path)
		filesInFolder, _ := os.ReadDir(folder)
		subs := []string{}
		for _, f := range filesInFolder {
			if f.IsDir() {
				continue
			}
			fname := f.Name()
			fext := strings.ToLower(filepath.Ext(fname))
			if fext != ".srt" && fext != ".vtt" {
				continue
			}
			if strings.Contains(strings.ToLower(fname), strings.ToLower(base)) {
				subRel, _ := filepath.Rel(dir, filepath.Join(folder, fname))
				subs = append(subs, filepath.ToSlash(subRel))
			}
		}

		item := MediaItem{
			ID:        id,
			Title:     base,
			RelPath:   relPath,
			Subtitles: subs,
		}

		mediaLibrary = append(mediaLibrary, item) // optional flat list
		catalogMap[catalogID] = append(catalogMap[catalogID], item)

		return nil
	})
}
func detectLang(sub string) string {
	// Example: "The.Traitors.India.S01E01.HINDI.1080p.H264-TheArmory.En.srt"
	base := strings.TrimSuffix(filepath.Base(sub), filepath.Ext(sub))
	parts := strings.Split(base, ".")
	last := parts[len(parts)-1]

	switch strings.ToLower(last) {
	case "en":
		return "English"
	case "bg":
		return "Bulgarian"
	default:
		return "Unknown"
	}
}

func generateTTID(path string) string {
	hash := md5.Sum([]byte(path))
	// take first 7 digits as a number
	num := int(binary.BigEndian.Uint32(hash[:4])) % 9999999
	return fmt.Sprintf("tt%07d", num)
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(data)
}
