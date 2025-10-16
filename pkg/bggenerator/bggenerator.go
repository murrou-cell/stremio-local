package bggenerator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/biter777/countries"
)

var tmdbAPIKey = os.Getenv("TMDB_API_KEY")

var cache = map[string]string{}

type TMDBResult struct {
	Results []struct {
		Name         string `json:"name"`
		Title        string `json:"title"`
		BackdropPath string `json:"backdrop_path"`
		PosterPath   string `json:"poster_path"`
	} `json:"results"`
}

func buildCountryRegex() *regexp.Regexp {
	var names []string
	for _, c := range countries.All() {
		info := c.Info()
		// Add main name
		names = append(names, regexp.QuoteMeta(strings.ToUpper(info.Name)))
		// Add "Common" English aliases
		if info.Name != "" {
			names = append(names, regexp.QuoteMeta(strings.ToUpper(info.Name)))
		}
	}

	// Add a few manually that are often used differently in releases
	names = append(names, "UK", "U\\.S\\.A", "U\\.S")

	// Build regex
	pattern := fmt.Sprintf(`(?i)\b(%s)\b`, strings.Join(names, "|"))
	return regexp.MustCompile(pattern)
}

func buildLanguageRegex() *regexp.Regexp {

	languages := []string{
		"ENGLISH", "HINDI", "FRENCH", "SPANISH", "GERMAN", "TURKISH",
		"KOREAN", "JAPANESE", "CHINESE", "ITALIAN", "RUSSIAN", "PORTUGUESE",
		"ARABIC", "DUTCH", "SWEDISH", "NORWEGIAN", "DANISH", "FINNISH",
		"POLISH", "GREEK", "CZECH", "HUNGARIAN", "ROMANIAN", "THAI",
	}
	return regexp.MustCompile(fmt.Sprintf(`(?i)\b(%s)\b`, strings.Join(languages, "|")))
}

func cleanTitle(raw string) string {
	title := raw
	// Remove extension
	title = regexp.MustCompile(`(?i)\.(mp4|mkv|avi|mov|webm)$`).ReplaceAllString(title, "")
	// Remove release group
	title = regexp.MustCompile(`-[A-Za-z0-9]+$`).ReplaceAllString(title, "")
	// Replace dots/underscores with spaces
	title = strings.NewReplacer(".", " ", "_", " ").Replace(title)
	// Remove technical tags
	remove := []string{
		`(?i)S\d{1,2}E\d{1,2}`, `(?i)E\d{1,2}`,
		`(?i)(480p|720p|1080p|2160p|4K|8K)`,
		`(?i)(x264|x265|H\.?264|H\.?265|HEVC|WEBRip|BluRay|HDRip|DVDRip|HDTV)`,
		buildLanguageRegex().String(),
		buildCountryRegex().String(),
		`(?i)\b(DUBBED|SUBBED|SUBS|DUB|MULTI)\b`,
		`(?i)\b(DD5\.1|AAC2\.0|MP3|FLAC|EAC3|TRUEHD|ATMOS)\b`,
		`(?i)\b(HDR|SDR|IMAX|REMASTERED|DIRECTORS CUT|EXTENDED|UNCUT)\b`,
		`(?i)\b(FINAL|PROPER|REPACK|LIMITED|INTERNAL)\b`,
		`(?i)\b(READNFO|NFO)\b`,
		`(?i)\b(UNRATED|THEATRICAL)\b`,
		`(?i)\b(NEWSEASON|SEASON|COMPLETE|FULL)\b`,
		`(?i)\b(HD)\b`,
		`(?i)\b(AMZN|NF|HULU|DISNEY\+|PRIME|NETFLIX)\b`,
		`(?i)\b(DDP5\.1|DD5\.1|DTS|DTS-HD|DTS:X|DTSMA)\b`,
		`(?i)\b(AC3|EVO|AVC|VC-1|VVC)\b`,
	}
	for _, r := range remove {
		title = regexp.MustCompile(r).ReplaceAllString(title, "")
	}
	// Normalize spaces
	title = regexp.MustCompile(`\s+`).ReplaceAllString(title, " ")
	return strings.TrimSpace(title)
}

func detectType(title string) string {
	// If filename contains season/episode markers → it's a TV show
	if regexp.MustCompile(`(?i)(S\d{1,2}E\d{1,2}|Season\s*\d+)`).MatchString(title) {
		return "tv"
	}
	return "movie"
}

func searchTMDB(title, mediaType string) (string, error) {
	query := url.QueryEscape(title)
	apiURL := fmt.Sprintf("https://api.themoviedb.org/3/search/%s?api_key=%s&query=%s", mediaType, tmdbAPIKey, query)
	resp, err := http.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var data TMDBResult
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	if len(data.Results) == 0 {
		return "", fmt.Errorf("no results for %s (%s)", title, mediaType)
	}

	base := "https://image.tmdb.org/t/p/original"
	r := data.Results[0]
	if r.BackdropPath != "" {
		return base + r.BackdropPath, nil
	}
	if r.PosterPath != "" {
		return base + r.PosterPath, nil
	}
	return "", fmt.Errorf("no image found for %s (%s)", title, mediaType)
}

func detectCountry(title string) string {
	upperTitle := strings.ToUpper(title)

	for _, country := range countries.All() {
		name := strings.ToUpper(country.Info().Name)
		if strings.Contains(upperTitle, name) {
			return country.Alpha2() // e.g. "IN", "US", "GB"
		}
	}

	return ""
}

func getBackgroundFromCache(title string) (string, bool) {
	result, found := cache[title]
	return result, found
}

func GenerateBackground(rawTitle string) string {
	cleaned := cleanTitle(rawTitle)
	if result, found := getBackgroundFromCache(cleaned); found {
		return result
	}
	searchTerm := cleaned

	mediaType := detectType(rawTitle)

	region := detectCountry(rawTitle)
	if region != "" {
		searchTerm = fmt.Sprintf("%s (%s)", cleaned, region)
	}

	// Try type-specific search first
	img, err := searchTMDB(searchTerm, mediaType)
	if err == nil {
		cache[cleaned] = img
		return img
	}

	// Fallback: try /multi (handles cross-type confusion)
	img, err = searchTMDB(cleaned, "multi")
	if err == nil {
		cache[cleaned] = img
		return img
	}

	// Final fallback: dummy image
	text := url.QueryEscape(cleaned)
	cache[cleaned] = fmt.Sprintf("https://dummyimage.com/1280x720/222222/ffffff&text=%s", text)
	return cache[cleaned]
}
