package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	state2 "github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/oni/tvx/lotus"
	"github.com/filecoin-project/oni/tvx/state"
)

var extractMsgCmd = &cli.Command{
	Name:        "generate-message",
	Description: "generate a message-class test vector",
	Flags:       []cli.Flag{&cidFlag, &apiFlag, &fileFlag},
	Action:      runExtractMsg,
}

func runExtractMsg(c *cli.Context) error {
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

	retain[builtin.RewardActorAddr] = struct{}{}

	fmt.Println("accessed actors:")
	for k := range retain {
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
	pretree, preroot, err := g.GetMaskedStateTree(prevTs.Parents(), retain)
	if err != nil {
		return err
	}

	driver := lotus.NewDriver(ctx)

	_, postroot, err := driver.ExecuteMessage(msg, preroot, pst.Blockstore, ts.Height())
	if err != nil {
		return fmt.Errorf("failed to execute message: %w", err)
	}

	posttree, err := state2.LoadStateTree(pst.CBORStore, postroot)
	if err != nil {
		return fmt.Errorf("failed to load post state tree: %w", err)
	}

	msgBytes, err := msg.Serialize()
	if err != nil {
		return err
	}

	// Serialize the trees.
	preData, preRoot, err := state.SerializeStateTree(ctx, pretree)
	if err != nil {
		return err
	}
	postData, postroot, err := state.SerializeStateTree(ctx, posttree)
	if err != nil {
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
				CAR:     preData,
				RootCID: preRoot.String(),
			},
		},
		ApplyMessage: msgBytes,
		Post: &Postconditions{
			StateTree: &StateTree{
				CAR:     postData,
				RootCID: postroot.String(),
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
