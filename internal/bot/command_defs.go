package bot

import (
	dg "github.com/bwmarrin/discordgo"
)

var (
	numConfessionsMinVal = 1.0

	Commands = []*dg.ApplicationCommand{
		{
			Name:        "confess",
			Description: "post anonymous confession",
			Options: []*dg.ApplicationCommandOption{

				{
					Type:        dg.ApplicationCommandOptionString,
					Name:        "text",
					Description: "confession body",
					Required:    true,
				},
			},
		},
		{
			Name:        "select-channel",
			Description: "choose confession channel target",
			Options: []*dg.ApplicationCommandOption{

				{
					Type:        dg.ApplicationCommandOptionChannel,
					Name:        "channel",
					Description: "target channel",
					Required:    true,
				},
			},
		},
		{
			Name:        "toggle-confessions",
			Description: "enable or disable confessions",
			Options: []*dg.ApplicationCommandOption{

				{
					Type:        dg.ApplicationCommandOptionBoolean,
					Name:        "open",
					Description: "True means confessions are open",
					Required:    true,
				},
			},
		},
		{
			Name:        "set-max-confessions",
			Description: "set the maximum number of confessions per user",
			Options: []*dg.ApplicationCommandOption{

				{
					Type:        dg.ApplicationCommandOptionInteger,
					Name:        "count",
					Description: "# of confessions allowd per user",
					MinValue:    &numConfessionsMinVal,
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
)
