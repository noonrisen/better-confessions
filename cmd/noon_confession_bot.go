package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"noon_confession_bot/internal/bot"
)

var (
	BotToken       = flag.String("token", "", "Bot access token")
	DeleteCommands = flag.Bool("rmcmd", true, "Remove all commands after shutdowning or not")
)

func main() {
	flag.Parse()

	bot := bot.NewBot(*BotToken, *DeleteCommands)
	bot.AddInteractionHandlers()
	bot.Login()
	bot.SetupCommands()

	defer bot.Session.Close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	log.Println("Press Ctrl+C to exit")
	<-stop

	if bot.DeleteCommands {
		log.Println("Deleting commands...")
		// We need to fetch the commands, since deleting requires the command ID.
		// We are doing this from the returned commands, because using
		// this will delete all the commands, which might not be desirable, so we
		// are deleting only the commands that we added.

		for _, v := range bot.RegisteredCommands {
			err := bot.Session.ApplicationCommandDelete(bot.Session.State.User.ID, "", v.ID)
			if err != nil {
				log.Printf("Cannot delete '%v' command: %v", v.Name, err)
			}
		}
	}

	log.Println("Gracefully shutting down.")
}
