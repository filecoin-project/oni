package main

import "github.com/urfave/cli/v2"

var (
	fromFlag = cli.StringFlag{
		Name:     "from",
		Usage:    "block CID of initial state",
		Required: true,
	}

	toFlag = cli.StringFlag{
		Name:     "to",
		Usage:    "block CID of ending state",
		Required: true,
	}

	apiFlag = cli.StringFlag{
		Name:    "api",
		Usage:   "api endpoint, formatted as token:multiaddr",
		Value:   "",
		EnvVars: []string{"FULLNODE_API_INFO"},
	}

	cidFlag = cli.StringFlag{
		Name:     "cid",
		Usage:    "message, block, or tipset CID",
		Required: false,
	}

	fileFlag = cli.StringFlag{
		Name:     "file",
		Usage:    "file",
		Required: true,
	}
)
