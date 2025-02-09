package tmdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	tmdbURL      = "https://api.themoviedb.org/3/%s"
	tmdbImageUrl = "https://image.tmdb.org/t/p/w94_and_h141_bestv2%s"
)

type Results struct {
	TmdbResults []TmdbResults `json:"results"`
}

type TmdbResults struct {
	BackdropPath     string  `json:"backdrop_path"`
	ID               int     `json:"id"`
	Title            string  `json:"title"`
	OriginalTitle    string  `json:"original_title"`
	Overview         string  `json:"overview"`
	PosterPath       string  `json:"poster_path"`
	MediaType        string  `json:"media_type"`
	Adult            bool    `json:"adult"`
	OriginalLanguage string  `json:"original_language"`
	GenreIds         []int   `json:"genre_ids"`
	Popularity       float64 `json:"popularity"`
	ReleaseDate      string  `json:"release_date"`
	Video            bool    `json:"video"`
	VoteAverage      float64 `json:"vote_average"`
	VoteCount        int     `json:"vote_count"`
}

type ShowDetails struct {
	Adult            bool          `json:"adult"`
	BackdropPath     interface{}   `json:"backdrop_path"`
	CreatedBy        []interface{} `json:"created_by"`
	EpisodeRunTime   []interface{} `json:"episode_run_time"`
	FirstAirDate     string        `json:"first_air_date"`
	Genres           []interface{} `json:"genres"`
	Homepage         string        `json:"homepage"`
	ID               int           `json:"id"`
	InProduction     bool          `json:"in_production"`
	Languages        []string      `json:"languages"`
	LastAirDate      interface{}   `json:"last_air_date"`
	LastEpisodeToAir interface{}   `json:"last_episode_to_air"`
	Name             string        `json:"name"`
	NextEpisodeToAir interface{}   `json:"next_episode_to_air"`
	Networks         []struct {
		ID            int    `json:"id"`
		LogoPath      string `json:"logo_path"`
		Name          string `json:"name"`
		OriginCountry string `json:"origin_country"`
	} `json:"networks"`
	NumberOfEpisodes    int           `json:"number_of_episodes"`
	NumberOfSeasons     int           `json:"number_of_seasons"`
	OriginCountry       []interface{} `json:"origin_country"`
	OriginalLanguage    string        `json:"original_language"`
	OriginalName        string        `json:"original_name"`
	Overview            string        `json:"overview"`
	Popularity          float64       `json:"popularity"`
	PosterPath          interface{}   `json:"poster_path"`
	ProductionCompanies []interface{} `json:"production_companies"`
	ProductionCountries []struct {
		Iso31661 string `json:"iso_3166_1"`
		Name     string `json:"name"`
	} `json:"production_countries"`
	Seasons         []interface{} `json:"seasons"`
	SpokenLanguages []struct {
		EnglishName string `json:"english_name"`
		Iso6391     string `json:"iso_639_1"`
		Name        string `json:"name"`
	} `json:"spoken_languages"`
	Status      string `json:"status"`
	Tagline     string `json:"tagline"`
	Type        string `json:"type"`
	VoteAverage int    `json:"vote_average"`
	VoteCount   int    `json:"vote_count"`
}


// https://api.themoviedb.org/3/search/multi?query=two%20towers&include_adult=false&language=en-US&page=1
func SearchItem(searchTerm, mediaType , tmdbApiKey string) (string, error) {
	// 1. Check the cache FIRST
	var tmdbUrl string
	searchTerm = strings.ReplaceAll(searchTerm, "ᴺᵉʷ", "")
	searchTerm = strings.ReplaceAll(searchTerm, "ᴸᶦᵛᵉ", "")
	searchTerm = strings.TrimSpace(searchTerm)

	// 2. If not in cache, make the TMDB request
	switch mediaType {
	case "SH":
		tmdbUrl = fmt.Sprintf(tmdbURL, "search/tv")
		mediaType = "SH"
	case "EP":
		tmdbUrl = fmt.Sprintf(tmdbURL, "search/tv")
		mediaType = "SH"
	case "MV":
		tmdbUrl = fmt.Sprintf(tmdbURL, "search/movie")
		mediaType = "MV"
	default: // Handle default/multi search case
		tmdbUrl = fmt.Sprintf(tmdbURL, "search/multi")
		mediaType = "default"
	}

	cachedURL, err := getImageURL(searchTerm + "-" + mediaType) // Include media type in cache key
	if err != nil {
		return "", fmt.Errorf("error checking cache: %w", err) // Handle cache read errors
	}

	if cachedURL != "" {
		return fmt.Sprintf(tmdbImageUrl, cachedURL), nil // Return cached URL if found
	}

	token := "Bearer " + tmdbApiKey

	req, err := http.NewRequest(http.MethodGet, tmdbUrl, nil) // Check for request creation errors
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	q := req.URL.Query()
	q.Add("query", searchTerm)
	q.Add("language", "en")
	q.Add("page", "1")

	req.Header.Set("Authorization", token)
	req.Header.Set("accept", "application/json")
	req.URL.RawQuery = q.Encode()

	client := &http.Client{}

	resp, err := client.Do(req)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tmdb response was non-200: %v", resp.Status)
	}
	if err != nil {
		return "", fmt.Errorf("error making TMDB request: %w", err)
	}
	defer resp.Body.Close()


	var r Results
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil { // Check decode errors
		return "", fmt.Errorf("error decoding TMDB response: %w", err)
	}

	if len(r.TmdbResults) == 0 {
		return "", fmt.Errorf("%s not found", mediaType)
	}

	posterPath := r.TmdbResults[0].PosterPath
	if posterPath == "" {
		return posterPath, nil
	}
	// 3. Add to cache AFTER successful TMDB request
	err = addImageToCache(searchTerm+"-"+mediaType, posterPath) // Use combined key
	if err != nil {
		fmt.Println("Error adding to cache:", err) // Log the error, but don't stop execution
	}

	return fmt.Sprintf(tmdbImageUrl, posterPath), nil
}

func getImageURL(name string) (string, error) {
	imageCacheFile, err := os.Open("image_cache.json") // Only open for reading
	if err != nil {
		if os.IsNotExist(err) { // Handle file not found gracefully.
			return "", nil // Treat as not found, no error
		}
		return "", fmt.Errorf("error opening image cache file: %w", err)
	}
	defer imageCacheFile.Close()

	var cache []map[string]string
	decoder := json.NewDecoder(imageCacheFile)

	if err := decoder.Decode(&cache); err != nil {
		if err != io.EOF {
			return "", fmt.Errorf("error decoding JSON: %w", err)
		}
	}

	for _, entry := range cache {
		if entry["name"] == name {
			return entry["url"], nil
		}
	}

	return "", nil // Not found
}

func addImageToCache(name, url string) error {
	entry := map[string]string{
		"name": name,
		"url":  url,
	}

	imageCacheFile, err := os.OpenFile("image_cache.json", os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("error opening image cache file: %w", err)
	}
	defer imageCacheFile.Close()

	var cache []map[string]string

	decoder := json.NewDecoder(imageCacheFile)

	if err := decoder.Decode(&cache); err != nil {
		if err != io.EOF {
			return fmt.Errorf("error decoding JSON: %w", err)
		}
	}

	// Check if the entry already exists (optional, but good practice).
	for _, existingEntry := range cache {
		if existingEntry["name"] == name && existingEntry["url"] == url {
			return nil // Already exists, no need to add.
		}
	}

	cache = append(cache, entry)

	// Seek to the beginning of the file to overwrite
	if _, err := imageCacheFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("error seeking to beginning of file: %w", err)
	}

	// Truncate the file to remove any old content.
	if err := imageCacheFile.Truncate(0); err != nil {
		return fmt.Errorf("error truncating file: %w", err)
	}

	encoder := json.NewEncoder(imageCacheFile)
	if err := encoder.Encode(cache); err != nil {
		return fmt.Errorf("error encoding JSON: %w", err)
	}

	return nil
}
