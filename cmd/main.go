package main

import (
	"fmt"
	"os"
	"strings"

	log "github.com/ChainSafe/log15"
	"github.com/mapprotocol/monitor/internal/config"
	"github.com/urfave/cli/v2"
)

var app = cli.NewApp()

var (
	Version   = "1.0.0"
	CommitID  = ""
	BuildTime = ""
)

func formatVersion(version, commitID, buildTime string) string {
	parts := make([]string, 0, 2)
	if commitID != "" {
		parts = append(parts, fmt.Sprintf("commitId=%s", commitID))
	}
	if buildTime != "" {
		parts = append(parts, fmt.Sprintf("buildTime=%s", buildTime))
	}
	if len(parts) == 0 {
		return version
	}
	return fmt.Sprintf("%s (%s)", version, strings.Join(parts, ", "))
}

// init initializes CLI
func init() {
	//app.Action = run
	app.Copyright = "Copyright 2021 MAP Protocol 2021 Authors"
	app.Name = "compass"
	app.Usage = "Compass"
	app.Authors = []*cli.Author{{Name: "MAP Protocol 2021"}}
	app.Version = formatVersion(Version, CommitID, BuildTime)
	app.EnableBashCompletion = true
	app.Commands = []*cli.Command{
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
