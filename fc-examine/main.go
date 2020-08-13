package main

import (
	"log"
	"os"
	"sort"

	"github.com/urfave/cli/v2"
)

var apiFlag = cli.StringFlag{
	Name:    "api",
	Usage:   "api endpoint, formatted as token:multiaddr",
	Value:   "",
	EnvVars: []string{"FULLNODE_API_INFO"},
}

func main() {
	app := &cli.App{
		Name:        "fc-examine",
		Description: "State Inspector 🕵️‍♂️",
		Commands: []*cli.Command{
			vectorCmd,
		},
	}

	sort.Sort(cli.CommandsByName(app.Commands))
	for _, c := range app.Commands {
		sort.Sort(cli.FlagsByName(c.Flags))
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
