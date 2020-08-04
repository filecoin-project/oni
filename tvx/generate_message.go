package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/oni/tvx/schema"
	"github.com/filecoin-project/oni/tvx/state"
)

var generateMessageCmd = &cli.Command{
	Name:        "generate-message",
	Description: "generate a message-class test vector",
	Flags:       []cli.Flag{&cidFlag, &apiFlag},
	Action:      runGenerateMessage,
}

func runGenerateMessage(c *cli.Context) error {
	node, err := makeClient(c.String(apiFlag.Name))
	if err != nil {
		return err
	}

	mid, err := cid.Decode(c.String(cidFlag.Name))
	if err != nil {
		return err
	}

	// Get actors accessed by message.
	actors, err := state.GetActorsForMessage(context.TODO(), node, mid)
	if err != nil {
		return err
	}

	actors[builtin.InitActorAddr] = struct{}{}

	fmt.Println("accessed actors:")
	for k := range actors {
		fmt.Println("\t", k.String())
	}

	// Filter the state root before and after.
	// TODO (raulk): I think we should calculate the after state by applying the
	//  message on top of the prestate, i.e. by _actually_ running the message
	//  using our reference implementation.
	//  Or we could have a CLI option for this.
	cache := state.NewReadThroughStore(context.TODO(), node)

	fmt.Println("getting the _before_ filtered state tree")
	preTree, err := state.GetFilteredStateTree(context.TODO(), node, cache, mid, actors, true)
	if err != nil {
		return err
	}

	fmt.Println("getting the _after_ filtered state tree")
	postTree, err := state.GetFilteredStateTree(context.TODO(), node, cache, mid, actors, false)
	if err != nil {
		return err
	}

	// Extract the serialized message itself.
	msg, err := node.ChainGetMessage(context.TODO(), mid)
	if err != nil {
		return err
	}
	msgBytes, err := msg.Serialize()
	if err != nil {
		return err
	}

	// Serialize the trees.
	preData, preRoot, err := state.SerializeStateTree(context.TODO(), preTree)
	if err != nil {
		return err
	}
	postData, postRoot, err := state.SerializeStateTree(context.TODO(), postTree)
	if err != nil {
		return err
	}

	version, err := node.Version(context.TODO())
	if err != nil {
		return err
	}

	// Write out the test vector.
	vector := schema.TestVector{
		Class:    schema.ClassMessage,
		Selector: "",
		Meta: &schema.Metadata{
			ID:      "TK",
			Version: "TK",
			Gen: schema.GenerationData{
				Source:  "TK",
				Version: version.String(),
			},
		},
		Pre: &schema.Preconditions{
			Epoch: 100,
			StateTree: &schema.StateTree{
				CAR:     preData,
				RootCID: preRoot.String(),
			},
		},
		ApplyMessage: msgBytes,
		Post: &schema.Postconditions{
			StateTree: &schema.StateTree{
				CAR:     postData,
				RootCID: postRoot.String(),
			},
		},
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&vector); err != nil {
		return err
	}

	return nil
}
