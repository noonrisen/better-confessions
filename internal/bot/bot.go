package bot

import (
	"log"
	"sync"

	dg "github.com/bwmarrin/discordgo"

	"noon_confession_bot/internal/utils"
)

type BotState struct {
	ConfessionChannelID     string
	PostCounter             map[string]uint
	LastConfessionMessageID string
	ConfessionNo            uint
	Active                  bool
	MaxPosts                uint
	RegisteredCommands      []*dg.ApplicationCommand
	Mu                      sync.Mutex
}

type Bot struct {
	State          *BotState
	Session        *dg.Session
	DeleteCommands bool
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

func NewBot(token string, deleteCommands bool) *Bot {
	var err error
	session, err := dg.New("Bot " + token)
	if err != nil {
		log.Fatalf("Invalid bot parameters: %v", err)
	}

	state := BotState{
		ConfessionChannelID:     "",
		PostCounter:             make(map[string]uint),
		LastConfessionMessageID: "",
		ConfessionNo:            0,
		Active:                  true,
		MaxPosts:                2,
		RegisteredCommands:      nil,
	}

	return &Bot{
		State:          &state,
		Session:        session,
		DeleteCommands: deleteCommands,
	}
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

	bot.State.Mu.Lock()
	defer bot.State.Mu.Unlock()

	bot.State.RegisteredCommands = make([]*dg.ApplicationCommand, len(commandHandlers))

	for i, v := range Commands {
		// empty string -> globally-registered commands
		cmd, err := s.ApplicationCommandCreate(s.State.User.ID, "", v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
		bot.State.RegisteredCommands[i] = cmd
	}
}
