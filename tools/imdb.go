package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type IMDBSearchResult struct {
	IMDBID string `json:"imdb_id"`
	Title  string `json:"title"`
	Year   string `json:"year"`
	Poster string `json:"poster"`
}

type IMDBTitle struct {
	ID                  string              `json:"id"`
	Title               string              `json:"title"`
	OgTitle             string              `json:"og_title"`
	Poster              string              `json:"poster"`
	AltTitle            string              `json:"alt_title"`
	Description         string              `json:"description"`
	Rating              float64             `json:"rating"`
	ViewerClass         string              `json:"viewer_class"`
	Duration            string              `json:"duration"`
	Genres              []string            `json:"genres"`
	ReleaseDate         string              `json:"release_date"`
	Actors              []string            `json:"actors"`
	Trailer             string              `json:"trailer"`
	CountryOfOrigin     string              `json:"country_of_origin"`
	Languages           string              `json:"languages"`
	AlsoKnownAs         string              `json:"also_known_as"`
	FilmingLocations    string              `json:"filming_locations"`
	ProductionCompanies string              `json:"production_companies"`
	RatingCount         string              `json:"rating_count"`
	MetaScore           string              `json:"meta_score"`
	MoreLikeThis        []MoreLikeThisEntry `json:"more_like_this"`
}

type MoreLikeThisEntry struct {
	IMDBID string `json:"imdb_id"`
	Title  string `json:"title"`
}

var IMDBSearch = &ToolDef{
	Name:        "imdb_search",
	Description: "Search IMDB for movies, TV shows, and actors. Returns top results with titles, years, and poster images.",
	Args: []ToolArg{
		{Name: "query", Description: "Search query (movie/show/actor name)", Required: true},
	},
	Execute: func(args map[string]string) string {
		query := strings.TrimSpace(args["query"])
		if query == "" {
			return jsonError("query required")
		}

		results, err := quickSearchImdb(query)
		if err != nil {
			return jsonError(fmt.Sprintf("search failed: %v", err))
		}

		b, _ := json.Marshal(results)
		return string(b)
	},
}

var IMDBGetTitle = &ToolDef{
	Name:        "imdb_title",
	Description: "Get detailed information about an IMDB title by ID (e.g., tt0111161 for Shawshank Redemption). Returns ratings, cast, genres, description, and more.",
	Args: []ToolArg{
		{Name: "title_id", Description: "IMDB title ID (e.g., tt0111161)", Required: true},
	},
	Execute: func(args map[string]string) string {
		titleID := strings.TrimSpace(args["title_id"])
		if titleID == "" {
			return jsonError("title_id required")
		}

		title, err := GetIMDBTitle(titleID)
		if err != nil {
			return jsonError(fmt.Sprintf("fetch failed: %v", err))
		}

		b, _ := json.Marshal(title)
		return string(b)
	},
}

func quickSearchImdb(query string) ([]IMDBSearchResult, error) {
	url := fmt.Sprintf("https://v3.sg.media-imdb.com/suggestion/x/%s.json?includeVideos=1", query)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var results struct {
		D []struct {
			ID string `json:"id"`
			L  string `json:"l"`
			Y  int    `json:"y"`
			I  struct {
				URL string `json:"imageUrl"`
			}
		} `json:"d"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	var searchResults []IMDBSearchResult
	for _, result := range results.D {
		searchResults = append(searchResults, IMDBSearchResult{
			IMDBID: result.ID,
			Title:  result.L,
			Year:   fmt.Sprintf("%d", result.Y),
			Poster: result.I.URL,
		})
	}

	return searchResults, nil
}

func GetIMDBTitle(titleID string) (*IMDBTitle, error) {
	url := fmt.Sprintf("https://www.imdb.com/title/%s/", titleID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch IMDb page: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	jsonMeta := doc.Find("script[type='application/ld+json']").First().Text()
	var jsonObj map[string]any
	if err := json.Unmarshal([]byte(jsonMeta), &jsonObj); err != nil {
		return nil, err
	}

	title := doc.Find("h1[data-testid=hero__pageTitle]").First().Text()
	poster, _ := jsonObj["image"].(string)
	description := getObjValue(jsonObj, "description")
	var rating = 0.0
	if jsonObj["aggregateRating"] != nil {
		if ratingObj, ok := jsonObj["aggregateRating"].(map[string]any); ok {
			if ratingValue, ok := ratingObj["ratingValue"].(float64); ok {
				rating = ratingValue
			}
		}
	}

	viewerClass, isViewerClass := jsonObj["contentRating"].(string)
	duration := doc.Find("li[data-testid=title-techspec_runtime] div").Text()
	genres := []string{}
	if genresArr, isGenres := jsonObj["genre"].([]any); isGenres {
		for _, genre := range genresArr {
			if g, ok := genre.(string); ok {
				genres = append(genres, g)
			}
		}
	}

	releaseDate := doc.Find("li[data-testid=title-details-releasedate] a").Text()
	actors := []string{}
	doc.Find("a[data-testid=title-cast-item__actor]").Each(func(i int, s *goquery.Selection) {
		actors = append(actors, s.Text())
	})

	trailer := ""
	if trailerObj, isTrailer := jsonObj["trailer"].(map[string]any); isTrailer {
		if embedURL, ok := trailerObj["embedUrl"].(string); ok {
			trailer = embedURL
		}
	}

	countryOfOrigin := ""
	doc.Find("li[data-testid=title-details-origin] a").Each(func(i int, s *goquery.Selection) {
		countryOfOrigin += s.Text() + ", "
	})
	countryOfOrigin = strings.TrimSuffix(countryOfOrigin, ", ")

	languages := ""
	doc.Find("li[data-testid=title-details-languages] a").Each(func(i int, s *goquery.Selection) {
		languages += s.Text() + ", "
	})
	languages = strings.TrimSuffix(languages, ", ")

	alsoKnownAs := doc.Find("li[data-testid=title-details-akas] div").First().Text()

	filmingLocations := ""
	doc.Find("li[data-testid=title-details-filminglocations] a").Each(func(i int, s *goquery.Selection) {
		filmingLocations += s.Text() + ", "
	})
	filmingLocations = strings.ReplaceAll(filmingLocations, "Filming locations, ", "")
	filmingLocations = strings.TrimSuffix(filmingLocations, ", ")

	productionCompanies := ""
	doc.Find("li[data-testid=title-details-companies] a").Each(func(i int, s *goquery.Selection) {
		productionCompanies += s.Text() + ", "
	})
	productionCompanies = strings.ReplaceAll(productionCompanies, "Production companies, ", "")
	productionCompanies = strings.TrimSuffix(productionCompanies, ", ")

	ratingCount := strings.ReplaceAll(doc.Find("div.sc-eb51e184-3").First().Text(), ",", "")
	altTitle, isAltTitle := jsonObj["alternateName"].(string)
	metaScore := doc.Find("span.metacritic-score-box").Text()

	moreLikeThis := []MoreLikeThisEntry{}
	doc.Find("section[data-testid=MoreLikeThis] div.ipc-poster-card").Each(func(i int, s *goquery.Selection) {
		mId, _ := s.Find("a.ipc-lockup-overlay").Attr("href")
		mId = strings.TrimPrefix(mId, "/title/")
		mId = strings.Split(mId, "/")[0]
		mTitle := s.Find("img.ipc-image").AttrOr("alt", "")
		moreLikeThis = append(moreLikeThis, MoreLikeThisEntry{
			IMDBID: mId,
			Title:  mTitle,
		})
	})

	tt := &IMDBTitle{
		ID:                  titleID,
		Title:               title,
		OgTitle:             doc.Find("div.sc-ec65ba05-1").First().Text(),
		Poster:              poster,
		Description:         description,
		Rating:              rating,
		Duration:            duration,
		Genres:              genres,
		ReleaseDate:         strings.Replace(releaseDate, "Release date", "", 1),
		Actors:              actors,
		Trailer:             trailer,
		CountryOfOrigin:     countryOfOrigin,
		Languages:           languages,
		AlsoKnownAs:         alsoKnownAs,
		FilmingLocations:    filmingLocations,
		ProductionCompanies: productionCompanies,
		RatingCount:         ratingCount,
		MetaScore:           metaScore,
		MoreLikeThis:        moreLikeThis,
	}

	if isAltTitle {
		tt.AltTitle = altTitle
	}
	if isViewerClass {
		tt.ViewerClass = viewerClass
	}

	return tt, nil
}

func getObjValue(obj map[string]any, key string) string {
	if val, exists := obj[key]; exists {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func jsonError(msg string) string {
	e := map[string]string{"error": msg}
	b, _ := json.Marshal(e)
	return string(b)
}
