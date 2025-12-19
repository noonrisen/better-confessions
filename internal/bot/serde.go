package bot

import (
	"encoding/gob"
	"log"
	"os"
	"strings"
	"sync"
)

// BotDataForSerialization is a structure that holds all the data we want to serialize
type BotDataForSerialization struct {
	GuildConfigs map[string]GuildConfigWithoutMutex
	Salt         []byte
}

type GuildConfigWithoutMutex struct {
	ConfessionChannelID     string
	PostCounter             map[string]uint
	LastConfessionMessageID string
	ConfessionNo            uint
	Active                  bool
	MaxPosts                uint
	CensoredWords           []string
}

// SaveData serializes GuildConfigs and Salt using Gob and writes it to a file.
func (b *Bot) SaveData(filename string) {
	file, err := os.Create(filename)
	if err != nil {
		log.Printf("Error creating file: %v", err)
		return
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)

	// Create a temporary map without mutexes
	tempMap := make(map[string]GuildConfigWithoutMutex)
	for k, v := range b.GuildConfigs {
		tempMap[k] = GuildConfigWithoutMutex{
			ConfessionChannelID:     v.ConfessionChannelID,
			PostCounter:             v.PostCounter,
			LastConfessionMessageID: v.LastConfessionMessageID,
			ConfessionNo:            v.ConfessionNo,
			Active:                  v.Active,
			MaxPosts:                v.MaxPosts,
			CensoredWords:           v.CensoredWords,
		}
	}

	botData := BotDataForSerialization{
		GuildConfigs: tempMap,
		Salt:         b.Salt,
	}

	err = encoder.Encode(botData)
	if err != nil {
		log.Printf("Error encoding data: %v", err)
		return
	}
	log.Printf("Saved state to: %s", filename)
}

// LoadData reads the Gob file and deserializes it into GuildConfigs and Salt.
func (b *Bot) LoadData(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("No existing data file found, starting fresh.")
			return
		}
		log.Printf("Error opening file: %v", err)
		return
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)

	var botData BotDataForSerialization
	err = decoder.Decode(&botData)
	if err != nil {
		log.Printf("Error decoding data: %v", err)
		return
	}

	// Convert back to GuildConfig with mutex
	b.GuildConfigs = make(map[string]*GuildConfig)
	for k, v := range botData.GuildConfigs {
		// Convert loaded words to lowercase (store original words on disk)
		lowerWords := make([]string, len(v.CensoredWords))
		for i, word := range v.CensoredWords {
			lowerWords[i] = strings.ToLower(word)
		}
		// Rebuild skeleton matcher (cached in memory only)
		matcher := buildSkeletonMatcher(lowerWords)

		b.GuildConfigs[k] = &GuildConfig{
			ConfessionChannelID:     v.ConfessionChannelID,
			PostCounter:             v.PostCounter,
			LastConfessionMessageID: v.LastConfessionMessageID,
			ConfessionNo:            v.ConfessionNo,
			Active:                  v.Active,
			MaxPosts:                v.MaxPosts,
			CensoredWords:           lowerWords,
			CensoredWordsMatcher:    matcher,
			Mu:                      sync.Mutex{},
		}
	}

	// Load the Salt
	b.Salt = botData.Salt
	log.Printf("Loaded state from: %s", filename)
}
