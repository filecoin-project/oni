package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/ipld/go-car"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/oni/tvx/lotus"
)

var execLotusFlags struct {
	file string
}

var execLotusCmd = &cli.Command{
	Name:        "exec-lotus",
	Description: "execute a test vector against Lotus",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "file",
			Usage:       "input file",
			Required:    true,
			Destination: &execLotusFlags.file,
		},
	},
	Action: runExecLotus,
}

func runExecLotus(_ *cli.Context) error {
	if execLotusFlags.file == "" {
		return fmt.Errorf("test vector file cannot be empty")
	}

	file, err := os.Open(execLotusFlags.file)
	if err != nil {
		return fmt.Errorf("failed to open test vector: %w", err)
	}

	var (
		dec = json.NewDecoder(file)
		tv  TestVector
	)

	if err = dec.Decode(&tv); err != nil {
		return fmt.Errorf("failed to decode test vector: %w", err)
	}

	switch tv.Class {
	case "message":
		var (
			ctx   = context.Background()
			epoch = tv.Pre.Epoch
		)

		bs := blockstore.NewTemporary()

		header, err := car.LoadCar(bs, bytes.NewReader(tv.Pre.StateTree.CAR))
		if err != nil {
			return fmt.Errorf("failed to load state tree car from test vector: %w", err)
		}

		fmt.Println("roots: ", header.Roots)

		fmt.Println("decoding message")
		msg, err := types.DecodeMessage(tv.ApplyMessage)
		if err != nil {
			return err
		}

		driver := lotus.NewDriver(ctx)

		fmt.Println("executing message")
		spew.Dump(driver.ExecuteMessage(msg, header.Roots[0], bs, epoch))

		return nil

	default:
		return fmt.Errorf("test vector class not supported")
	}
}
