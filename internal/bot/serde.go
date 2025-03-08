package bot

import (
    "encoding/gob"
    "os"
    "log"
    "sync"
)

// BotDataForSerialization is a structure that holds all the data we want to serialize
type BotDataForSerialization struct {
    GuildConfigs map[string]GuildConfigWithoutMutex
    Salt         []byte
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
        }
    }

    botData := BotDataForSerialization{
        GuildConfigs: tempMap,
        Salt:         b.Salt,
    }

    err = encoder.Encode(botData)
    if err != nil {
        log.Printf("Error encoding data: %v", err)
    }
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

    // Load the Salt
    b.Salt = botData.Salt
}

// GuildConfigWithoutMutex remains the same as before
type GuildConfigWithoutMutex struct {
    ConfessionChannelID     string
    PostCounter             map[string]uint
    LastConfessionMessageID string
    ConfessionNo            uint
    Active                  bool
    MaxPosts                uint
}

