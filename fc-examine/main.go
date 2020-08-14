package main

import (
	"log"
	"os"
	"sort"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:        "fc-examine",
		Description: "State Inspector 🕵️‍♂️",
		Commands: []*cli.Command{
			vectorCmd,
			diffCmd,
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
