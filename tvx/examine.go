package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/oni/tvx/examine"
	"github.com/filecoin-project/oni/tvx/state"
)

var examineFlags struct {
	file string
}

var examineCmd = &cli.Command{
	Name:        "examine",
	Description: "examine an exported state root as represented in a test vector",
	Action:      runExamineCmd,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "file",
			Usage:       "test vector file",
			Required:    true,
			Destination: &examineFlags.file,
		},
	},
}

func runExamineCmd(_ *cli.Context) error {
	file, err := os.Open(examineFlags.file)
	if err != nil {
		return err
	}

	var tv TestVector
	if err := json.NewDecoder(file).Decode(&tv); err != nil {
		return err
	}

	encoded, err := state.RecoverStore(context.TODO(), tv.CAR)
	if err != nil {
		return err
	}

	examine.Diff(
		context.TODO(),
		encoded.Blockstore,
		tv.Pre.StateTree.RootCID,
		tv.Pre.StateTree.RootCID,
		tv.Post.StateTree.RootCID)

	return nil
}

