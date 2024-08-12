package main

import (
    // "errors"
    "flag"
    "fmt"
    "log"
    "os"
    "sync"
    "os/signal"
    "crypto/sha256"
    "encoding/base64"
    "crypto/rand"

    "github.com/bwmarrin/discordgo"
)

const MAX_POSTS = 5

// Bot parameters
var (
    GuildID        = flag.String("guild", "", "Test guild ID. If not passed - bot registers commands globally")
    BotToken       = flag.String("token", "", "Bot access token")
    DeleteCommands = flag.Bool("rmcmd", true, "Remove all commands after shutdowning or not")
    ConfessionChannelID  = flag.String("channel", "", "Target Channel ID")
)
func init() { flag.Parse() }

type PostCounter struct {
    postCounts map[string]int
    mu         sync.Mutex
}

func init() {
    var err error
    s, err = discordgo.New("Bot " + *BotToken)
    if err != nil {
        log.Fatalf("Invalid bot parameters: %v", err)
    }
    fmt.Print("s:\n", s)
}

var s *discordgo.Session

var (
    dmPermission                   = false
    defaultMemberPermissions int64 = discordgo.PermissionManageServer
    lastConfessionMessageID string
    salt = generateRandomSalt()

    counter = &PostCounter{
        postCounts: make(map[string]int),
    }

    commands = []*discordgo.ApplicationCommand{
        {
            Name:        "confess",
            Description: "post anonymous confession",
            Options: []*discordgo.ApplicationCommandOption{

                {
                    Type:        discordgo.ApplicationCommandOptionString,
                    Name:        "text",
                    Description: "confession body",
                    Required:    true,
                },
            },
        },

        /***************************************************************

        {
            Name:        "select-channel",
            Description: "choose confession channel target",
            Options: []*discordgo.ApplicationCommandOption{

                {
                    Type:        discordgo.ApplicationCommandOptionChannel,
                    Name:        "channel",
                    Description: "target channel",
                    Required:    true,
                },
            },
        },
        {
            Name:        "toggle-confessions",
            Description: "enable or disable confessions",
            Options: []*discordgo.ApplicationCommandOption{

                {
                    Type:        discordgo.ApplicationCommandOptionBoolean,
                    Name:        "open",
                    Description: "True means confessions are open",
                    Required:    true,
                },
            },
        },

        ********************************************************************/

    }

    commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
        "confess": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
            sendEphemeralMessage := func(content string) {
                s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
                    Type: discordgo.InteractionResponseChannelMessageWithSource,
                    Data: &discordgo.InteractionResponseData{
                        Flags:   discordgo.MessageFlagsEphemeral,
                        Content: content,
                    },
                })
            }

            confession := i.ApplicationCommandData().Options[0].StringValue()
            userID := i.Member.User.ID
            guildID := i.GuildID

            err := processConfession(s, confession, userID, guildID)
            if err != nil {
                if err.Error() == "you have exceeded the maximum number of allowed posts" {
                    sendEphemeralMessage(":x: You have exceeded the maximum number of allowed posts.")
                } else {
                    sendEphemeralMessage(fmt.Sprintf(":x: There was an error processing your confession: %s", err))
                }
                return
            }

            sendEphemeralMessage(":white_check_mark: Your confession has been posted.")
        },

    }
)

func generateRandomSalt() []byte {
    salt := make([]byte, 16)
    _, err := rand.Read(salt)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to generate salt: %v\n", err)
        os.Exit(1)
    }
    return salt
}

func processConfession(s *discordgo.Session, confession string, userID, guildID string) error {
    // 1. Check # of posts
    allowed, err := checkPostLimit(guildID, userID)
    if err != nil {
        return fmt.Errorf("error checking post limit: %w", err)
    }

    if !allowed {
        return fmt.Errorf("you have exceeded the maximum number of allowed posts")
    }

    // 2. Post anonymously
    _, err = s.ChannelMessageSendComplex(*ConfessionChannelID, &discordgo.MessageSend{
        Embeds: []*discordgo.MessageEmbed{
            {
                Title:       "Confession # <NUMBER>",
                Description: confession,
            },
        },
        Components: []discordgo.MessageComponent{
            discordgo.ActionsRow{
                Components: []discordgo.MessageComponent{
                    discordgo.Button{
                        Label:    "Submit a confession!",
                        CustomID: "confess_button",
                        Style:    discordgo.PrimaryButton,
                    },
                },
            },
        },
    })

    if err != nil {
        return fmt.Errorf("error posting confession: %w", err)
    }

    return nil
}

func handleConfessionModalSubmit(s *discordgo.Session, i *discordgo.InteractionCreate) {
    if i.Type == discordgo.InteractionModalSubmit && i.ModalSubmitData().CustomID == "confession_modal" {
        confession := i.ModalSubmitData().Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
        userID := i.Member.User.ID
        guildID := i.GuildID

        err := processConfession(s, confession, userID, guildID)
        if err != nil {
            s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
                Type: discordgo.InteractionResponseChannelMessageWithSource,
                Data: &discordgo.InteractionResponseData{
                    Flags:   discordgo.MessageFlagsEphemeral,
                    Content: fmt.Sprintf(":x: There was an error processing your confession: %s", err),
                },
            })
            return
        }

        s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
            Type: discordgo.InteractionResponseChannelMessageWithSource,
            Data: &discordgo.InteractionResponseData{
                Flags:   discordgo.MessageFlagsEphemeral,
                Content: ":white_check_mark: Your confession has been posted.",
            },
        })
    }
}


// Generates a secure*** key by hashing guildID and userID with a salt
func generateSecureKey(guildID, userID string) string {
    data := fmt.Sprintf("%s:%s", guildID, userID)
    saltedData := append([]byte(data), salt...)
    hash := sha256.Sum256(saltedData)
    return base64.StdEncoding.EncodeToString(hash[:])
}

// Function to check the post limit for a user
func checkPostLimit(guildID, userID string) (bool, error) {
    secureKey := generateSecureKey(guildID, userID)

    counter.mu.Lock()
    defer counter.mu.Unlock()

    count := counter.postCounts[secureKey]
    fmt.Println("COUNT:", count)

    if count >= MAX_POSTS {
        return false, nil
    }

    counter.postCounts[secureKey]++
    return true, nil
}

func handleConfessButtonClick(s *discordgo.Session, i *discordgo.InteractionCreate) {
    if i.Type == discordgo.InteractionMessageComponent && i.MessageComponentData().CustomID == "confess_button" {
        modal := discordgo.InteractionResponse{
            Type: discordgo.InteractionResponseModal,
            Data: &discordgo.InteractionResponseData{
                CustomID: "confession_modal",
                Title:    "Submit Your Confession",
                Components: []discordgo.MessageComponent{
                    discordgo.ActionsRow{
                        Components: []discordgo.MessageComponent{
                            discordgo.TextInput{
                                Label:       "Your Confession",
                                CustomID:    "confession_input",
                                Style:       discordgo.TextInputParagraph,
                                Placeholder: "Type your confession here...",
                                Required:    true,
                            },
                        },
                    },
                },
            },
        }

        err := s.InteractionRespond(i.Interaction, &modal)
        if err != nil {
            log.Println("Failed to send modal:", err)
        }
    }
}

func init() {
    s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        switch i.Type {
        case discordgo.InteractionApplicationCommand:
            // Handle slash commands
            if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
                h(s, i)
            }
        case discordgo.InteractionMessageComponent:
            // Handle button clicks
            if i.MessageComponentData().CustomID == "confess_button" {
                handleConfessButtonClick(s, i)
            }
        case discordgo.InteractionModalSubmit:
            // Handle modal submissions
            if i.ModalSubmitData().CustomID == "confession_modal" {
                handleConfessionModalSubmit(s, i)
            }
        }
    })
}


func main() {
    s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
        log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
    })
    err := s.Open()
    if err != nil {
        log.Fatalf("Cannot open the session: %v", err)
    }

    log.Println("Creating commands...")
    registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
    for i, v := range commands {
        cmd, err := s.ApplicationCommandCreate(s.State.User.ID, *GuildID, v)
        if err != nil {
            log.Panicf("Cannot create '%v' command: %v", v.Name, err)
        }
        registeredCommands[i] = cmd
    }

    defer s.Close()

    stop := make(chan os.Signal, 1)
    signal.Notify(stop, os.Interrupt)
    log.Println("Press Ctrl+C to exit")
    <-stop

    if *DeleteCommands {
        log.Println("Deleting commands...")
        // We need to fetch the commands, since deleting requires the command ID.
        // We are doing this from the returned commands, because using
        // this will delete all the commands, which might not be desirable, so we
        // are deleting only the commands that we added.

        for _, v := range registeredCommands {
            err := s.ApplicationCommandDelete(s.State.User.ID, *GuildID, v.ID)
            if err != nil {
                log.Panicf("Cannot delete '%v' command: %v", v.Name, err)
            }
        }
    }

    log.Println("Gracefully shutting down.")
}
