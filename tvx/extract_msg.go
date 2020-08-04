package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/oni/tvx/lotus"
	"github.com/filecoin-project/oni/tvx/state"
)

var extractMsgCmd = &cli.Command{
	Name:        "extract-message",
	Description: "generate a message-class test vector by extracting it from a network",
	Flags:       []cli.Flag{&cidFlag, &apiFlag, &fileFlag},
	Action:      runExtractMsg,
}

func runExtractMsg(c *cli.Context) error {
	_ = os.Setenv("LOTUS_DISABLE_VM_BUF", "iknowitsabadidea")

	ctx := context.Background()

	// get the output file.
	outputFile := c.String(fileFlag.Name)
	if outputFile == "" {
		return fmt.Errorf("output file required")
	}

	mid, err := cid.Decode(c.String(cidFlag.Name))
	if err != nil {
		return err
	}

	// Make the client.
	api, err := makeClient(c.String(apiFlag.Name))
	if err != nil {
		return err
	}

	// locate the message.
	msgInfo, err := api.StateSearchMsg(ctx, mid)
	if err != nil {
		return fmt.Errorf("failed to locate message: %w", err)
	}

	// Extract the serialized message.
	msg, err := api.ChainGetMessage(ctx, mid)
	if err != nil {
		return err
	}

	// create a read through store that uses ChainGetObject to fetch unknown CIDs.
	pst := state.NewProxyingStore(ctx, api)

	g := state.NewSurgeon(ctx, api, pst)

	// Get actors accessed by message.
	retain, err := g.GetAccessedActors(ctx, api, mid)
	if err != nil {
		return err
	}

	retain = append(retain, builtin.RewardActorAddr)

	fmt.Println("accessed actors:")
	for _, k := range retain {
		fmt.Println("\t", k.String())
	}

	// get the tipset on which this message was mined.
	ts, err := api.ChainGetTipSet(ctx, msgInfo.TipSet)
	if err != nil {
		return err
	}

	// get the previous tipset, on top of which the message was executed.
	prevTs, err := api.ChainGetTipSet(ctx, ts.Parents())
	if err != nil {
		return err
	}

	fmt.Println("getting the _before_ filtered state tree")
	preroot, err := g.GetMaskedStateTree(prevTs.Parents(), retain)
	if err != nil {
		return err
	}

	driver := lotus.NewDriver(ctx)

	_, postroot, err := driver.ExecuteMessage(msg, preroot, pst.Blockstore, ts.Height())
	if err != nil {
		return fmt.Errorf("failed to execute message: %w", err)
	}

	msgBytes, err := msg.Serialize()
	if err != nil {
		return err
	}

	preout := new(bytes.Buffer)
	if err := g.WriteCAR(preout, preroot); err != nil {
		return err
	}

	postout := new(bytes.Buffer)
	if err := g.WriteCAR(postout, postroot); err != nil {
		return err
	}

	version, err := api.Version(ctx)
	if err != nil {
		return err
	}

	// Write out the test vector.
	vector := TestVector{
		Class:    ClassMessage,
		Selector: "",
		Meta: &Metadata{
			ID:      "TK",
			Version: "TK",
			Gen: GenerationData{
				Source:  "TK",
				Version: version.String(),
			},
		},
		Pre: &Preconditions{
			Epoch: ts.Height(),
			StateTree: &StateTree{
				CAR: preout.Bytes(),
			},
		},
		ApplyMessage: msgBytes,
		Post: &Postconditions{
			StateTree: &StateTree{
				CAR: postout.Bytes(),
			},
		},
	}

	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&vector); err != nil {
		return err
	}

	return nil
}