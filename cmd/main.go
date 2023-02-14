package main

import (
	log "github.com/ChainSafe/log15"
	"github.com/mapprotocol/monitor/internal/config"
	"github.com/urfave/cli/v2"
	"os"
)

var app = cli.NewApp()

var accountCommand = cli.Command{
	Name:  "accounts",
	Usage: "manage bridge keystore",
	Description: "The accounts command is used to manage the bridge keystore.\n" +
		"\tTo generate a new account (key type generated is determined on the flag passed in): compass accounts generate\n" +
		"\tTo import a keystore file: compass accounts import path/to/file\n" +
		"\tTo import a geth keystore file: compass accounts import --ethereum path/to/file\n" +
		"\tTo import a private key file: compass accounts import --privateKey private_key\n" +
		"\tTo list keys: compass accounts list",
	Subcommands: []*cli.Command{
		{
			Action: wrapHandler(handleImportCmd),
			Name:   "import",
			Usage:  "import bridge keystore",
			Flags:  config.FlagsOfImportCmd,
			Description: "The import subcommand is used to import a keystore for the bridge.\n" +
				"\tA path to the keystore must be provided\n" +
				"\tUse --ethereum to import an ethereum keystore from external sources such as geth\n" +
				"\tUse --privateKey to create a keystore from a provided private key.",
		},
	},
}

var (
	Version = "1.0.0"
)

// init initializes CLI
func init() {
	//app.Action = run
	app.Copyright = "Copyright 2021 MAP Protocol 2021 Authors"
	app.Name = "compass"
	app.Usage = "Compass"
	app.Authors = []*cli.Author{{Name: "MAP Protocol 2021"}}
	app.Version = Version
	app.EnableBashCompletion = true
	app.Commands = []*cli.Command{
		&accountCommand,
		&monitorCommand,
	}

	app.Flags = append(app.Flags, config.VerbosityFlag)
}

func main() {
	if err := app.Run(os.Args); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}
