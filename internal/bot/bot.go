package bot

import (
	"log"
	"sync"

	dg "github.com/bwmarrin/discordgo"

	"noon_confession_bot/internal/utils"
)

type GuildConfig struct {
	ConfessionChannelID     string
	PostCounter             map[string]uint
	LastConfessionMessageID string
	ConfessionNo            uint
	Active                  bool
	MaxPosts                uint
	Mu                      sync.Mutex
}

type Bot struct {
	Session            *dg.Session
	DeleteCommands     bool
	RegisteredCommands []*dg.ApplicationCommand
	GuildConfigs       map[string]*GuildConfig
}

var (
	salt = utils.GenerateRandomSalt()

	commandHandlers = map[string]func(b *Bot, s *dg.Session, i *dg.InteractionCreate){
		"confess":             (*Bot).confessHandler,
		"select-channel":      (*Bot).selectChannelHandler,
		"toggle-confessions":  (*Bot).toggleConfessionsHandler,
		"set-max-confessions": (*Bot).setMaxConfessionsHandler,
		"reset-post-counter":  (*Bot).resetPostCounterHandler,
	}
)

func NewGuildConfig() GuildConfig {
	return GuildConfig{
		ConfessionChannelID:     "",
		PostCounter:             make(map[string]uint),
		LastConfessionMessageID: "",
		ConfessionNo:            0,
		Active:                  true,
		MaxPosts:                2,
		// the mutex's 0 value is already functional
	}
}

func NewBot(token string, deleteCommands bool) *Bot {
	var err error
	session, err := dg.New("Bot " + token)
	if err != nil {
		log.Fatalf("Invalid bot parameters: %v", err)
	}

	return &Bot{
		Session:            session,
		DeleteCommands:     deleteCommands,
		RegisteredCommands: nil,
		GuildConfigs:       make(map[string]*GuildConfig),
	}
}

func (b *Bot) GetGuildCfg(guildID string) *GuildConfig {
	gc, ok := b.GuildConfigs[guildID]
	if ok {
		return gc
	}

	newGuild := NewGuildConfig()
	b.GuildConfigs[guildID] = &newGuild
	log.Print("New guild: ", guildID)
	return &newGuild
}

func (bot *Bot) Login() {
	bot.Session.AddHandler(func(s *dg.Session, r *dg.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})

	err := bot.Session.Open()

	if err != nil {
		log.Fatalf("Cannot open the session: %v", err)
	}
}

func (b *Bot) AddInteractionHandlers() {
	b.Session.AddHandler(func(s *dg.Session, i *dg.InteractionCreate) {
		switch i.Type {
		case dg.InteractionApplicationCommand:
			// Handle slash commands
			if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
				h(b, s, i)
			}
		case dg.InteractionMessageComponent:
			// Handle button clicks
			if i.MessageComponentData().CustomID == "confess_button" {
				confessButtonClickHandler(s, i)
			}
		case dg.InteractionModalSubmit:
			// Handle modal submissions
			if i.ModalSubmitData().CustomID == "confession_modal" {
				b.confessionModalHandler(s, i)
			}
		}
	})
}

func (bot *Bot) SetupCommands() {
	log.Println("Creating commands...")
	s := bot.Session

	bot.RegisteredCommands = make([]*dg.ApplicationCommand, len(commandHandlers))

	for i, v := range Commands {
		// empty string -> globally-registered commands
		cmd, err := s.ApplicationCommandCreate(s.State.User.ID, "", v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
		bot.RegisteredCommands[i] = cmd
	}
}
