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

// Bot parameters
var (
    GuildID        = flag.String("guild", "", "Test guild ID. If not passed - bot registers commands globally")
    BotToken       = flag.String("token", "", "Bot access token")
    DeleteCommands = flag.Bool("rmcmd", true, "Remove all commands after shutdowning or not")
    ConfessionChannelID  = flag.String("channel", "", "Target Channel ID")
)
func init() { flag.Parse() }

type BotState struct {
    PostCounter             map[string]uint
    LastConfessionMessageID string
    ConfessionNo            uint
    Active                  bool
    MaxPosts                uint
    mu                      sync.Mutex
}

func init() {
    var err error
    s, err = discordgo.New("Bot " + *BotToken)
    if err != nil {
        log.Fatalf("Invalid bot parameters: %v", err)
    }
}

var s *discordgo.Session

var (
    dmPermission                   = false
    defaultMemberPermissions int64 = discordgo.PermissionManageServer
    salt = generateRandomSalt()

    botState = BotState{
        PostCounter:               make(map[string]uint),
        LastConfessionMessageID:   "",
        ConfessionNo:              0,
        Active:                    true,
        MaxPosts:                  2,
    }

    integerOptionMinValue          = 1.0
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
        {
            Name:        "set-max-confessions",
            Description: "set the maximum number of confessions per user",
            Options: []*discordgo.ApplicationCommandOption{

                {
                    Type:        discordgo.ApplicationCommandOptionInteger,
                    Name:        "count",
                    Description: "# of confessions allowd per user",
                    MinValue:    &integerOptionMinValue,
                    MaxValue:    1 << 31,
                    Required:    true,
                },
            },
        },
        {
            Name:        "reset-post-counter",
            Description: "Allow everyone to confess again, i.e. reset the post counter.",
        },
    }

    commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
        "confess": func(s *discordgo.Session, i *discordgo.InteractionCreate) {

            confession := i.ApplicationCommandData().Options[0].StringValue()
            userID := i.Member.User.ID
            guildID := i.GuildID

            err := processConfession(s, confession, userID, guildID)
            if err != nil {
                if err.Error() == "you have exceeded the maximum number of allowed posts" {
                    sendEphemeralMessage(s, i, ":x: You have exceeded the maximum number of allowed posts.")
                } else {
                    sendEphemeralMessage(s, i, fmt.Sprintf(":x: There was an error processing your confession: %s", err))
                }
                return
            }

            sendEphemeralMessage(s, i, ":white_check_mark: Your confession has been posted.")
        },

        "select-channel": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
            if !hasPermission(s, i) {
                sendEphemeralMessage(s, i, ":x: You can't do that.")
                return
            }

            botState.mu.Lock()
            defer botState.mu.Unlock()

            // Use ChannelValue() to get the selected channel
            selectedChannel := i.ApplicationCommandData().Options[0].ChannelValue(s)

            // Set ConfessionChannelID to the selected channel's ID
            ConfessionChannelID = &selectedChannel.ID
            botState.LastConfessionMessageID = ""

            sendEphemeralMessage(s, i, ":white_check_mark: Channel updated.")
        },


        "toggle-confessions": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
            if !hasPermission(s, i) {
                sendEphemeralMessage(s, i, ":x: You can't do that.")
                return
            }
            userOption := i.ApplicationCommandData().Options[0].Value

            // Attempt to cast the value to a bool
            userBool, ok := userOption.(bool)
            if !ok {
                // Handle the error case where the value is not a bool
                sendEphemeralMessage(s, i, ":x: problem reading boolean")
                return
            }

            botState.mu.Lock()
            defer botState.mu.Unlock()
            botState.Active = userBool
            sendEphemeralMessage(s, i, fmt.Sprintf("Taking confessions: %t", userBool))
        },
        "set-max-confessions": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
            if !hasPermission(s, i) {
                sendEphemeralMessage(s, i, ":x: You can't do that.")
                return
            }
            userInt := i.ApplicationCommandData().Options[0].IntValue()

            botState.mu.Lock()
            defer botState.mu.Unlock()
            botState.MaxPosts = uint(userInt)
            sendEphemeralMessage(s, i, fmt.Sprintf("Max # of posts allowed is now: %d", botState.MaxPosts))
            return
        },
        "reset-post-counter": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
            if !hasPermission(s, i) {
                sendEphemeralMessage(s, i, ":x: You can't do that.")
                return
            }
            botState.mu.Lock()
            defer botState.mu.Unlock()
            botState.PostCounter = make(map[string]uint)
            sendEphemeralMessage(s, i, fmt.Sprintf(":white_check_mark: Reset complete."))
            return
        },
    }
)

func sendEphemeralMessage(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
    s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
        Type: discordgo.InteractionResponseChannelMessageWithSource,
        Data: &discordgo.InteractionResponseData{
            Flags:   discordgo.MessageFlagsEphemeral,
            Content: content,
        },
    })
}

func generateRandomSalt() []byte {
    salt := make([]byte, 16)
    _, err := rand.Read(salt)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to generate salt: %v\n", err)
        os.Exit(1)
    }
    return salt
}


func checkState() error {
    botState.mu.Lock()
    defer botState.mu.Unlock()

    if !botState.Active && *ConfessionChannelID == "" {
        return fmt.Errorf("The bot is not active now. Also, the target channel is not set.")
    } else if !botState.Active {
        return fmt.Errorf("The bot is not active now.")
    } else if *ConfessionChannelID == ""  {
        return fmt.Errorf("The target channel is not set.")
    }
    return nil
}

func hasPermission(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
    // Fetch the guild details
    guild, err := s.State.Guild(i.GuildID)
    if err != nil {
        log.Println("Error fetching guild:", err)
        return false
    }

    // Check if the user is the server owner
    if i.Member.User.ID == guild.OwnerID {
        return true
    }

    // Fetch the user's permissions in the guild
    permissions, err := s.State.UserChannelPermissions(i.Member.User.ID, i.ChannelID)
    if err != nil {
        log.Println("Error fetching permissions:", err)
        return false
    }

    // Check for Administrator or ManageServer permissions
    return permissions&discordgo.PermissionAdministrator != 0
}


func processConfession(s *discordgo.Session, confession string, userID, guildID string) error {

    if err := checkState(); err != nil {
        return err
    }

    // 1. Check # of posts
    allowed, err := checkPostLimit(guildID, userID)
    if err != nil {
        return fmt.Errorf("error checking post limit: %w", err)
    }

    if !allowed {
        return fmt.Errorf("you have exceeded the maximum number of allowed posts")
    }

    botState.mu.Lock()
    defer botState.mu.Unlock()

    // 2. Edit the last confession message to remove its button
    if botState.LastConfessionMessageID != "" {

        // Fetch the message to get its current content and embeds
        message, err := s.ChannelMessage(*ConfessionChannelID, botState.LastConfessionMessageID)
        if err != nil {
            return fmt.Errorf("error fetching previous confession message: %w", err)
        }

        // Edit the message, keeping the same content and embeds but removing the components
        _, err = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
            ID:         botState.LastConfessionMessageID,
            Channel:    *ConfessionChannelID,
            Content:    &message.Content, // Use the current content
            Embeds:     &message.Embeds,   // Keep the current embeds
            Components: &[]discordgo.MessageComponent{}, // Remove the components (buttons)
        })
        if err != nil {
            return fmt.Errorf("error removing button from previous confession: %w", err)
        }
    }


    // 3. Post the new confession anonymously
    msg, err := s.ChannelMessageSendComplex(*ConfessionChannelID, &discordgo.MessageSend{
        Embeds: []*discordgo.MessageEmbed{
            {
                Title:       fmt.Sprintf("Confession #%d", botState.ConfessionNo),
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

    // 4. Store the message ID of the new confession
    botState.LastConfessionMessageID = msg.ID
    botState.ConfessionNo++

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

    botState.mu.Lock()
    defer botState.mu.Unlock()

    count := botState.PostCounter[secureKey]

    if count >= botState.MaxPosts {
        return false, nil
    }

    botState.PostCounter[secureKey]++
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
