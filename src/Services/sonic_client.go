package Services

import (
	"cuento-backend/config"
	"log"

	"github.com/expectedsh/go-sonic/sonic"
)

const SonicCollection = "cuento"

const (
	SonicBucketGamePosts    = "game_posts"
	SonicBucketGeneralPosts = "general_posts"
	SonicBucketLorePosts    = "lore_posts"
	SonicBucketCharacters   = "characters"
	SonicBucketWantedPosts  = "wanted_posts"
	SonicBucketEpisodes     = "episodes"
)

var sonicCfg *config.SonicConfig

func InitSonic() {
	cfg := config.LoadSonicConfig()

	// Verify connectivity via a control connection
	c, err := sonic.NewControl(cfg.Host, cfg.Port, cfg.Password)
	if err != nil {
		log.Printf("Warning: could not connect to Sonic at %s:%d — search will be unavailable: %v", cfg.Host, cfg.Port, err)
		return
	}
	_ = c.Quit()

	sonicCfg = cfg
	log.Printf("Successfully connected to Sonic at %s:%d", cfg.Host, cfg.Port)
}

func SonicAvailable() bool {
	return sonicCfg != nil
}

// SonicPush indexes text for an object in the given collection and bucket.
func SonicPush(collection, bucket, objectID, text string, lang sonic.Lang) error {
	c, err := sonic.NewIngester(sonicCfg.Host, sonicCfg.Port, sonicCfg.Password)
	if err != nil {
		return err
	}
	defer c.Quit()
	return c.Push(collection, bucket, objectID, text, lang)
}

// SonicDelete removes all indexed text for an object from a collection/bucket.
func SonicDelete(collection, bucket, objectID string) error {
	c, err := sonic.NewIngester(sonicCfg.Host, sonicCfg.Port, sonicCfg.Password)
	if err != nil {
		return err
	}
	defer c.Quit()
	return c.FlushObject(collection, bucket, objectID)
}

// SonicQuery searches for a term in the given collection and bucket.
// Returns a slice of object IDs.
func SonicQuery(collection, bucket, term string, limit, offset int, lang sonic.Lang) ([]string, error) {
	c, err := sonic.NewSearch(sonicCfg.Host, sonicCfg.Port, sonicCfg.Password)
	if err != nil {
		return nil, err
	}
	defer c.Quit()
	return c.Query(collection, bucket, term, limit, offset, lang)
}

// SonicSuggest returns autocomplete suggestions for a word prefix.
func SonicSuggest(collection, bucket, word string, limit int) ([]string, error) {
	c, err := sonic.NewSearch(sonicCfg.Host, sonicCfg.Port, sonicCfg.Password)
	if err != nil {
		return nil, err
	}
	defer c.Quit()
	return c.Suggest(collection, bucket, word, limit)
}
