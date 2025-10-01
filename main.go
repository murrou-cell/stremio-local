package main

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type MediaItem struct {
	ID        string
	Title     string
	RelPath   string
	Subtitles []string
}

var mediaDir = "/media" // root folder with subfolders

var catalogMap = map[string][]MediaItem{}
var mediaMap = map[string]MediaItem{} // map fake ttID -> item

func withCORS(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		handler(w, r)
	}
}

func main() {
	if len(os.Args) > 1 {
		mediaDir = os.Args[1]
	}
	scanMediaDir(mediaDir)

	http.HandleFunc("/manifest.json", withCORS(manifestHandler))
	http.HandleFunc("/catalog/movie/", withCORS(catalogHandler))
	http.HandleFunc("/stream/movie/", withCORS(streamHandler))
	http.HandleFunc("/subtitles/movie/", withCORS(subtitlesHandler))
	http.HandleFunc("/files/", withCORS(filesHandler))
	http.HandleFunc("/meta/movie/", withCORS(metaHandler))

	log.Println("Addon listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}

func generatePoster(title string) string {
	text := url.QueryEscape(title)
	width, height := 200, 300
	bgColor := "444444"   // dark gray background
	textColor := "ffffff" // white text

	// Example: https://dummyimage.com/200x300/444444/ffffff&text=The+Traitors
	return fmt.Sprintf("https://dummyimage.com/%dx%d/%s/%s&text=%s", width, height, bgColor, textColor, text)
}

func generateBackground(title string) string {
	text := url.QueryEscape(title)
	width, height := 1280, 720
	bgColor := "222222"
	textColor := "ffffff"

	return fmt.Sprintf("https://dummyimage.com/%dx%d/%s/%s&text=%s", width, height, bgColor, textColor, text)
}

func metaHandler(w http.ResponseWriter, r *http.Request) {
	// URL: /meta/movie/<ttID>.json
	path := strings.TrimPrefix(r.URL.Path, "/meta/movie/")
	path = strings.TrimSuffix(path, ".json")
	id := path

	item, ok := mediaMap[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]interface{}{"meta": nil})
		return
	}

	meta := map[string]interface{}{
		"id":         item.ID,
		"type":       "movie",
		"name":       item.Title,
		"poster":     generatePoster(item.Title),
		"background": generateBackground(item.Title),
		"year":       2025, // optional: extract from filename if needed
		"plot":       "Local movie served via Stremio addon",
		"genres":     []string{"Local"},
	}

	writeJSON(w, map[string]interface{}{"meta": meta})
}

// -------------------- Media Scan --------------------

func scanMediaDir(dir string) {
	catalogMap = map[string][]MediaItem{}
	mediaMap = map[string]MediaItem{}

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		if ext != ".mp4" && ext != ".mkv" {
			return nil
		}

		base := strings.TrimSuffix(info.Name(), ext)
		relPath, _ := filepath.Rel(dir, path)
		relPath = filepath.ToSlash(relPath)

		// Fake ttID
		id := generateTTID(relPath)

		// Catalog = top-level folder
		parts := strings.Split(relPath, "/")
		catalogID := parts[0]

		// Scan subtitles in same folder
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

		mediaMap[id] = item
		catalogMap[catalogID] = append(catalogMap[catalogID], item)

		return nil
	})
	log.Println("Scan complete, catalogs:", len(catalogMap))
}

// -------------------- Fake ttID --------------------

func generateTTID(path string) string {
	hash := md5.Sum([]byte(path))
	num := int(binary.BigEndian.Uint32(hash[:4])) % 9999999
	return fmt.Sprintf("%07d", num)
}

// -------------------- Manifest --------------------

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

// -------------------- Catalog --------------------

func catalogHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Catalog request:", r.URL.Path)
	path := strings.TrimPrefix(r.URL.Path, "/catalog/movie/")
	path = strings.TrimSuffix(path, ".json")
	catalogID := path

	items, ok := catalogMap[catalogID]
	if !ok {
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
			"poster": generatePoster(item.Title),
		})
	}
	log.Println("Returning", metas)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"metas": metas})
}

// -------------------- Stream --------------------
func absURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host // automatically includes port if needed
	return fmt.Sprintf("%s://%s/%s", scheme, host, path)
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/stream/movie/")
	path = strings.TrimSuffix(path, ".json")
	id := path

	item, ok := mediaMap[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]interface{}{"streams": []interface{}{}})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"streams": []map[string]string{
			{
				"title": item.Title,
				"url":   absURL(r, "files/"+item.RelPath), // dynamic URL
			},
		},
	})
}

// -------------------- Subtitles --------------------

func detectLang(sub string) string {
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

func subtitlesHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/subtitles/movie/")
	idx := strings.Index(path, ".json")
	if idx != -1 {
		path = path[:idx]
	}
	parts := strings.Split(path, "/filename=")
	id := parts[0]

	item, ok := mediaMap[id]
	if !ok {
		writeJSON(w, map[string]interface{}{"subtitles": []interface{}{}})
		return
	}

	subs := []map[string]string{}
	for _, s := range item.Subtitles {
		subs = append(subs, map[string]string{
			"id":   strings.TrimSuffix(filepath.Base(s), filepath.Ext(s)),
			"url":  absURL(r, "files/"+s), // dynamic URL
			"lang": detectLang(s),
		})
	}
	writeJSON(w, map[string]interface{}{"subtitles": subs})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(v)
}

// -------------------- File server --------------------

func filesHandler(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, ".srt") {
		w.Header().Set("Content-Type", "application/x-subrip")
	}
	if strings.HasSuffix(r.URL.Path, ".vtt") {
		w.Header().Set("Content-Type", "text/vtt")
	}
	http.StripPrefix("/files/", http.FileServer(http.Dir(mediaDir))).ServeHTTP(w, r)
}
