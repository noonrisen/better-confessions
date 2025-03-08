package bot

import (
	"encoding/gob"
	"log"
	"os"
	"sync"
)

// SaveData serializes GuildConfigs using Gob and writes it to a file.
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
		}
	}

	err = encoder.Encode(tempMap)
	if err != nil {
		log.Printf("Error encoding data: %v", err)
		return
	}
	log.Printf("Saved state: %s", filename)
}

// LoadData reads the Gob file and deserializes it into GuildConfigs.
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

	var tempMap map[string]GuildConfigWithoutMutex
	err = decoder.Decode(&tempMap)
	if err != nil {
		log.Printf("Error decoding data: %v", err)
		return
	}

	// Convert back to GuildConfig with mutex
	b.GuildConfigs = make(map[string]*GuildConfig)
	for k, v := range tempMap {
		b.GuildConfigs[k] = &GuildConfig{
			ConfessionChannelID:     v.ConfessionChannelID,
			PostCounter:             v.PostCounter,
			LastConfessionMessageID: v.LastConfessionMessageID,
			ConfessionNo:            v.ConfessionNo,
			Active:                  v.Active,
			MaxPosts:                v.MaxPosts,
			Mu:                      sync.Mutex{},
		}
	}
	log.Printf("Loaded state from: %s", filename)
}

// GuildConfigWithoutMutex is a version of GuildConfig without the mutex for serialization
type GuildConfigWithoutMutex struct {
	ConfessionChannelID     string
	PostCounter             map[string]uint
	LastConfessionMessageID string
	ConfessionNo            uint
	Active                  bool
	MaxPosts                uint
}
