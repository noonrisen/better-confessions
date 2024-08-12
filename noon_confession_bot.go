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

const MAX_POSTS = 1

// Bot parameters
var (
    GuildID        = flag.String("guild", "", "Test guild ID. If not passed - bot registers commands globally")
    BotToken       = flag.String("token", "", "Bot access token")
    DeleteCommands = flag.Bool("rmcmd", true, "Remove all commands after shutdowning or not")
    ConfessionChannelID  = flag.String("channel", "", "Target Channel ID")
)

type PostCounter struct {
    postCounts map[string]int
    mu         sync.Mutex
}

var s *discordgo.Session

func init() { flag.Parse() }

func init() {
    var err error
    s, err = discordgo.New("Bot " + *BotToken)
    if err != nil {
        log.Fatalf("Invalid bot parameters: %v", err)
    }
    fmt.Print("s:\n", s)
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

var (
    salt = generateRandomSalt()
    counter = &PostCounter{
        postCounts: make(map[string]int),
    }
    dmPermission                   = false
    defaultMemberPermissions int64 = discordgo.PermissionManageServer

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

            // 1. check # of posts
            guildID := i.GuildID
            fmt.Println("Guild ID:", guildID)
            userID := i.Member.User.ID
            fmt.Println("userID :", userID)

            fmt.Println("checking post limit...")
            allowed, err := checkPostLimit(guildID, userID)

            if err != nil {
                sendEphemeralMessage(":x: There was an error checking your post limit.")
                return
            }

            if !allowed {
                sendEphemeralMessage(":x: You have exceeded the maximum number of allowed posts.")
                return
            }

            // 2. post anonymously
            fmt.Println("posting confession as bot")

            confession := i.ApplicationCommandData().Options[0].StringValue()

            _, err = s.ChannelMessageSend(*ConfessionChannelID, confession)

            if err != nil {
                sendEphemeralMessage(":x: There was an error posting your confession.")
                return
            }

            // 3. successful followup (this appears to the invoking user only)
            sendEphemeralMessage(":white_check_mark: Your confession has been posted.")
            return

        },
    }
)

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

func init() {
    s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
        if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
            h(s, i)
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
