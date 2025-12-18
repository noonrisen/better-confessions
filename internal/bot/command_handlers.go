package bot

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"noon_confession_bot/internal/utils"
	"regexp"
	"strings"

	dg "github.com/bwmarrin/discordgo"
	"github.com/cloudflare/ahocorasick"
)

//
// Internals/Helpers
//

func checkInteractionMemberNil(i *dg.InteractionCreate) error {
	if i == nil {
		log.Print("got nil interaction in confessHandler")
		return errors.New("got nil interaction in confessHandler")
	}

	if i.Member == nil || i.Member.User == nil {
		log.Print("got nil interaction Member component in confessHandler")
		log.Print("i:", i)
		return errors.New("got nil interaction Member component in confessHandler")
	}
	return nil
}

func (b Bot) generateSecureKey(guildID, userID string) string {
	data := fmt.Sprintf("%s:%s", guildID, userID)
	saltedData := append([]byte(data), b.Salt...)
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

func checkState(gc *GuildConfig) error {

	if !gc.Active && gc.ConfessionChannelID == "" {
		return fmt.Errorf("The bot is not active now. Also, the target channel is not set.")
	} else if !gc.Active {
		return fmt.Errorf("The bot is not active now.")
	} else if gc.ConfessionChannelID == "" {
		return fmt.Errorf("The target channel is not set.")
	}
	return nil
}

// Function to check the post limit for a user
func (b *Bot) checkIncrementPostLimit(guildID, userID string) (bool, error) {
	secureKey := b.generateSecureKey(guildID, userID)

	guildCfg := b.GetGuildCfg(guildID)

	count := guildCfg.PostCounter[secureKey]

	if count >= guildCfg.MaxPosts {
		return false, nil
	}

	guildCfg.PostCounter[secureKey]++
	return true, nil
}

func editLastMessage(s *dg.Session, guildCfg *GuildConfig) {
	if guildCfg.LastConfessionMessageID == "" {
		return
	}

	// Fetch the message to get its current content and embeds
	message, err := s.ChannelMessage(guildCfg.ConfessionChannelID, guildCfg.LastConfessionMessageID)
	if err != nil {
		log.Printf("error fetching previous confession message: %s", err.Error())
		return
	}

	// if the last message was just a url, we do not want to mess up the embed.
	embed := &message.Embeds
	if _, err := utils.Url(message.Content); err == nil {
		embed = nil
	}

	// Edit the message, keeping the same content and embeds but removing the components
	_, err = s.ChannelMessageEditComplex(&dg.MessageEdit{
		ID:         guildCfg.LastConfessionMessageID,
		Channel:    guildCfg.ConfessionChannelID,
		Content:    &message.Content,         // Use the current content
		Embeds:     embed,                    // Keep the current embeds
		Components: &[]dg.MessageComponent{}, // Remove the components (buttons)
	})
	if err != nil {
		log.Printf("error removing button from previous confession: %s", err.Error())
	}
}

func (b *Bot) processConfession(s *dg.Session, confession string, userID, guildID string) error {
	guildCfg := b.GetGuildCfg(guildID)

	guildCfg.Mu.Lock()
	defer guildCfg.Mu.Unlock()

	if err := checkState(guildCfg); err != nil {
		return err
	}

	// 1. Check # of posts
	if allowed, err := b.checkIncrementPostLimit(guildID, userID); err != nil {
		return fmt.Errorf("error checking post limit: %w", err)
	} else if !allowed {
		return fmt.Errorf("you have exceeded the maximum number of allowed posts")
	}

	// 2. Check for censored words
	if guildCfg.CensoredWordsMatcher != nil {
		confessionLower := strings.ToLower(confession)
		matches := guildCfg.CensoredWordsMatcher.Match([]byte(confessionLower))
		if len(matches) > 0 {
			return fmt.Errorf("your confession contains words that are not allowed")
		}
	}

	// Trim excess newlines
	re := regexp.MustCompile(`\n{3,}`)
	confession = re.ReplaceAllString(confession, "\n\n")

	// 2. Edit the last confession message to remove its button
	editLastMessage(s, guildCfg)

	// 3. Post the new confession anonymously
	var msg *dg.Message
	var postErr error
	if url, err := utils.Url(confession); err == nil {
		// if the confession is just a url, post just that
		msg, postErr = s.ChannelMessageSendComplex(guildCfg.ConfessionChannelID, &dg.MessageSend{
			Content:    *url,
			Components: ConfessButtonMessageComponent,
		})
	} else {
		msg, postErr = s.ChannelMessageSendComplex(guildCfg.ConfessionChannelID, &dg.MessageSend{
			Embeds: []*dg.MessageEmbed{
				{
					Title:       fmt.Sprintf("Confession #%d", guildCfg.ConfessionNo),
					Description: confession,
				},
			},
			Components: ConfessButtonMessageComponent,
		})
	}

	if postErr != nil {
		return fmt.Errorf("error posting confession: %w", postErr)
	}

	// 4. Store the message ID of the new confession
	guildCfg.LastConfessionMessageID = msg.ID
	guildCfg.ConfessionNo++

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

	if err := checkInteractionMemberNil(i); err != nil {
		sendEphemeralMessage(s, i, "internal problem: tag noon if you want to help")
		return
	}

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
	guildID := i.GuildID
	guildCfg := b.GetGuildCfg(guildID)

	if !hasPermission(s, i) {
		sendEphemeralMessage(s, i, ":x: You can't do that.")
		return
	}

	guildCfg.Mu.Lock()
	defer guildCfg.Mu.Unlock()

	// Use ChannelValue() to get the selected channel
	selectedChannel := i.ApplicationCommandData().Options[0].ChannelValue(s)

	// Set ConfessionChannelID to the selected channel's ID
	guildCfg.ConfessionChannelID = selectedChannel.ID
	guildCfg.LastConfessionMessageID = ""

	sendEphemeralMessage(s, i, ":white_check_mark: Channel updated.")
}

func (b *Bot) toggleConfessionsHandler(s *dg.Session, i *dg.InteractionCreate) {
	guildID := i.GuildID
	guildCfg := b.GetGuildCfg(guildID)

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

	guildCfg.Mu.Lock()
	defer guildCfg.Mu.Unlock()

	guildCfg.Active = userBool
	sendEphemeralMessage(s, i, fmt.Sprintf("Taking confessions: %t", userBool))
}

func (b *Bot) setMaxConfessionsHandler(s *dg.Session, i *dg.InteractionCreate) {
	guildID := i.GuildID
	guildCfg := b.GetGuildCfg(guildID)

	if !hasPermission(s, i) {
		sendEphemeralMessage(s, i, ":x: You can't do that.")
		return
	}
	userInt := i.ApplicationCommandData().Options[0].IntValue()

	guildCfg.Mu.Lock()
	defer guildCfg.Mu.Unlock()
	guildCfg.MaxPosts = uint(userInt)
	sendEphemeralMessage(s, i, fmt.Sprintf("Max # of posts allowed is now: %d", guildCfg.MaxPosts))
}

func (b *Bot) resetPostCounterHandler(s *dg.Session, i *dg.InteractionCreate) {
	guildID := i.GuildID
	guildCfg := b.GetGuildCfg(guildID)

	if !hasPermission(s, i) {
		sendEphemeralMessage(s, i, ":x: You can't do that.")
		return
	}
	guildCfg.Mu.Lock()
	defer guildCfg.Mu.Unlock()
	guildCfg.PostCounter = make(map[string]uint)
	sendEphemeralMessage(s, i, ":white_check_mark: Reset complete.")
}

func (b *Bot) setCensoredWordsHandler(s *dg.Session, i *dg.InteractionCreate) {
	guildID := i.GuildID
	guildCfg := b.GetGuildCfg(guildID)

	if !hasPermission(s, i) {
		sendEphemeralMessage(s, i, ":x: You can't do that.")
		return
	}

	guildCfg.Mu.Lock()
	currentWords := strings.Join(guildCfg.CensoredWords, ", ")
	guildCfg.Mu.Unlock()

	modal := dg.InteractionResponse{
		Type: dg.InteractionResponseModal,
		Data: &dg.InteractionResponseData{
			CustomID: "censored_words_modal",
			Title:    "Set Censored Words",
			Components: []dg.MessageComponent{
				dg.ActionsRow{
					Components: []dg.MessageComponent{
						dg.TextInput{
							Label:       "Censored Words (comma separated)",
							CustomID:    "censored_words_input",
							Style:       dg.TextInputParagraph,
							MinLength:   0,
							MaxLength:   4000,
							Placeholder: "word1, word2, word3",
							Value:       currentWords,
							Required:    false,
						},
					},
				},
			},
		},
	}

	err := s.InteractionRespond(i.Interaction, &modal)
	if err != nil {
		log.Println("Failed to send censored words modal:", err)
		sendEphemeralMessage(s, i, ":x: Failed to open modal.")
	}
}

func (b *Bot) censoredWordsModalHandler(s *dg.Session, i *dg.InteractionCreate) {
	guildID := i.GuildID
	guildCfg := b.GetGuildCfg(guildID)

	if !hasPermission(s, i) {
		sendEphemeralMessage(s, i, ":x: You can't do that.")
		return
	}

	wordsInput := i.ModalSubmitData().Components[0].(*dg.ActionsRow).Components[0].(*dg.TextInput).Value

	// Parse comma-separated words, trim whitespace, convert to lowercase, and filter out empty strings
	words := []string{}
	if wordsInput != "" {
		parts := strings.Split(wordsInput, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				// Convert to lowercase when storing
				words = append(words, strings.ToLower(trimmed))
			}
		}
	}

	guildCfg.Mu.Lock()
	guildCfg.CensoredWords = words
	// Rebuild the matcher with the new words
	if len(words) > 0 {
		guildCfg.CensoredWordsMatcher = ahocorasick.NewStringMatcher(words)
	} else {
		guildCfg.CensoredWordsMatcher = nil
	}
	guildCfg.Mu.Unlock()

	s.InteractionRespond(i.Interaction, &dg.InteractionResponse{
		Type: dg.InteractionResponseChannelMessageWithSource,
		Data: &dg.InteractionResponseData{
			Flags:   dg.MessageFlagsEphemeral,
			Content: fmt.Sprintf(":white_check_mark: Censored words updated. (%d words)", len(words)),
		},
	})
}

func hasPermission(s *dg.Session, i *dg.InteractionCreate) bool {
	// Fetch the guild details
	guild, err := s.State.Guild(i.GuildID)
	if err != nil {
		log.Println("Error fetching guild:", err)
		return false
	}

	if err := checkInteractionMemberNil(i); err != nil {
		sendEphemeralMessage(s, i, "internal problem: tag noon if you want to help")
		return false
	}

	// Check if the user is the server owner
	if i.Member.User.ID == guild.OwnerID {
		return true
	}

	// Fetch the user's permissions in the guild
	permissions := i.Member.Permissions

	// Check for Administrator or ManageServer permissions
	return permissions&dg.PermissionAdministrator != 0
}
