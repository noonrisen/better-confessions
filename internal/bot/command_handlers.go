package bot

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"regexp"

	dg "github.com/bwmarrin/discordgo"
)

//
// Internals/Helpers
//

func generateSecureKey(guildID, userID string) string {
	data := fmt.Sprintf("%s:%s", guildID, userID)
	saltedData := append([]byte(data), salt...)
	hash := sha256.Sum256(saltedData)
	return base64.StdEncoding.EncodeToString(hash[:])
}

func sendEphemeralMessage(s *dg.Session, i *dg.InteractionCreate, content string) {
	s.InteractionRespond(i.Interaction, &dg.InteractionResponse{
		Type: dg.InteractionResponseChannelMessageWithSource,
		Data: &dg.InteractionResponseData{
			Flags:   dg.MessageFlagsEphemeral,
			Content: content,
		},
	})
}

func checkState(s *BotState) error {

	if !s.Active && s.ConfessionChannelID == "" {
		return fmt.Errorf("The bot is not active now. Also, the target channel is not set.")
	} else if !s.Active {
		return fmt.Errorf("The bot is not active now.")
	} else if s.ConfessionChannelID == "" {
		return fmt.Errorf("The target channel is not set.")
	}
	return nil
}

// Function to check the post limit for a user
func (b *Bot) checkPostLimit(guildID, userID string) (bool, error) {
	secureKey := generateSecureKey(guildID, userID)

	count := b.State.PostCounter[secureKey]

	if count >= b.State.MaxPosts {
		return false, nil
	}

	b.State.PostCounter[secureKey]++
	return true, nil
}

func (b *Bot) processConfession(s *dg.Session, confession string, userID, guildID string) error {

	b.State.Mu.Lock()
	defer b.State.Mu.Unlock()

	if err := checkState(b.State); err != nil {
		return err
	}

	// 1. Check # of posts
	allowed, err := b.checkPostLimit(guildID, userID)
	if err != nil {
		return fmt.Errorf("error checking post limit: %w", err)
	}

	if !allowed {
		return fmt.Errorf("you have exceeded the maximum number of allowed posts")
	}

	// Trim excess newlines
	re := regexp.MustCompile(`\n{3,}`)
	confession = re.ReplaceAllString(confession, "\n\n")

	// 2. Edit the last confession message to remove its button
	if b.State.LastConfessionMessageID != "" {

		// Fetch the message to get its current content and embeds
		message, err := s.ChannelMessage(b.State.ConfessionChannelID, b.State.LastConfessionMessageID)
		if err != nil {
			return fmt.Errorf("error fetching previous confession message: %w", err)
		}

		// Edit the message, keeping the same content and embeds but removing the components
		_, err = s.ChannelMessageEditComplex(&dg.MessageEdit{
			ID:         b.State.LastConfessionMessageID,
			Channel:    b.State.ConfessionChannelID,
			Content:    &message.Content,         // Use the current content
			Embeds:     &message.Embeds,          // Keep the current embeds
			Components: &[]dg.MessageComponent{}, // Remove the components (buttons)
		})
		if err != nil {
			return fmt.Errorf("error removing button from previous confession: %w", err)
		}
	}

	// 3. Post the new confession anonymously
	msg, err := s.ChannelMessageSendComplex(b.State.ConfessionChannelID, &dg.MessageSend{
		Embeds: []*dg.MessageEmbed{
			{
				Title:       fmt.Sprintf("Confession #%d", b.State.ConfessionNo),
				Description: confession,
			},
		},
		Components: []dg.MessageComponent{
			dg.ActionsRow{
				Components: []dg.MessageComponent{
					dg.Button{
						Label:    "Submit a confession!",
						CustomID: "confess_button",
						Style:    dg.PrimaryButton,
					},
				},
			},
		},
	})

	if err != nil {
		return fmt.Errorf("error posting confession: %w", err)
	}

	// 4. Store the message ID of the new confession
	b.State.LastConfessionMessageID = msg.ID
	b.State.ConfessionNo++

	return nil
}

//
// Modal/Button Interaction Handlers
//

func (b *Bot) confessionModalHandler(s *dg.Session, i *dg.InteractionCreate) {
	confession := i.ModalSubmitData().Components[0].(*dg.ActionsRow).Components[0].(*dg.TextInput).Value
	userID := i.Member.User.ID
	guildID := i.GuildID

	err := b.processConfession(s, confession, userID, guildID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &dg.InteractionResponse{
			Type: dg.InteractionResponseChannelMessageWithSource,
			Data: &dg.InteractionResponseData{
				Flags:   dg.MessageFlagsEphemeral,
				Content: fmt.Sprintf(":x: There was an error processing your confession: %s", err),
			},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &dg.InteractionResponse{
		Type: dg.InteractionResponseChannelMessageWithSource,
		Data: &dg.InteractionResponseData{
			Flags:   dg.MessageFlagsEphemeral,
			Content: ":white_check_mark: Your confession has been posted.",
		},
	})
}

func confessButtonClickHandler(s *dg.Session, i *dg.InteractionCreate) {
	modal := dg.InteractionResponse{
		Type: dg.InteractionResponseModal,
		Data: &dg.InteractionResponseData{
			CustomID: "confession_modal",
			Title:    "Submit Your Confession",
			Components: []dg.MessageComponent{
				dg.ActionsRow{
					Components: []dg.MessageComponent{
						dg.TextInput{
							Label:       "Your Confession",
							CustomID:    "confession_input",
							Style:       dg.TextInputParagraph,
							MinLength:   1,
							MaxLength:   1024,
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

//
// slash command handlers
//

func (b *Bot) confessHandler(s *dg.Session, i *dg.InteractionCreate) {

	confession := i.ApplicationCommandData().Options[0].StringValue()
	userID := i.Member.User.ID
	guildID := i.GuildID

	err := b.processConfession(s, confession, userID, guildID)
	if err != nil {
		if err.Error() == "you have exceeded the maximum number of allowed posts" {
			sendEphemeralMessage(s, i, ":x: You have exceeded the maximum number of allowed posts.")
		} else {
			sendEphemeralMessage(s, i, fmt.Sprintf(":x: There was an error processing your confession: %s", err))
		}
		return
	}

	sendEphemeralMessage(s, i, ":white_check_mark: Your confession has been posted.")
}

func (b *Bot) selectChannelHandler(s *dg.Session, i *dg.InteractionCreate) {
	if !hasPermission(s, i) {
		sendEphemeralMessage(s, i, ":x: You can't do that.")
		return
	}

	b.State.Mu.Lock()
	defer b.State.Mu.Unlock()

	// Use ChannelValue() to get the selected channel
	selectedChannel := i.ApplicationCommandData().Options[0].ChannelValue(s)

	// Set ConfessionChannelID to the selected channel's ID
	b.State.ConfessionChannelID = selectedChannel.ID
	b.State.LastConfessionMessageID = ""

	sendEphemeralMessage(s, i, ":white_check_mark: Channel updated.")
	log.Print("ChanID: ", b.State.ConfessionChannelID)
}

func (b *Bot) toggleConfessionsHandler(s *dg.Session, i *dg.InteractionCreate) {
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

	b.State.Mu.Lock()
	defer b.State.Mu.Unlock()

	b.State.Active = userBool
	sendEphemeralMessage(s, i, fmt.Sprintf("Taking confessions: %t", userBool))
}

func (b *Bot) setMaxConfessionsHandler(s *dg.Session, i *dg.InteractionCreate) {
	if !hasPermission(s, i) {
		sendEphemeralMessage(s, i, ":x: You can't do that.")
		return
	}
	userInt := i.ApplicationCommandData().Options[0].IntValue()

	b.State.Mu.Lock()
	defer b.State.Mu.Unlock()
	b.State.MaxPosts = uint(userInt)
	sendEphemeralMessage(s, i, fmt.Sprintf("Max # of posts allowed is now: %d", b.State.MaxPosts))
	return
}

func (b *Bot) resetPostCounterHandler(s *dg.Session, i *dg.InteractionCreate) {
	if !hasPermission(s, i) {
		sendEphemeralMessage(s, i, ":x: You can't do that.")
		return
	}
	b.State.Mu.Lock()
	defer b.State.Mu.Unlock()
	b.State.PostCounter = make(map[string]uint)
	sendEphemeralMessage(s, i, fmt.Sprintf(":white_check_mark: Reset complete."))
	return
}

func hasPermission(s *dg.Session, i *dg.InteractionCreate) bool {
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
	return permissions&dg.PermissionAdministrator != 0
}
