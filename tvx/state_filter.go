package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/oni/tvx/state"
)

var filterCmd = &cli.Command{
	Name:        "filter-state",
	Description: "filter a state tree to retain only actors affected by a message",
	Flags:       []cli.Flag{&cidFlag, &apiFlag},
	Action:      runFilter,
}

func runFilter(c *cli.Context) error {
	node, err := makeClient(c.String(apiFlag.Name))
	if err != nil {
		return err
	}

	mid, err := cid.Decode(c.String(cidFlag.Name))
	if err != nil {
		return err
	}

	cache := state.NewReadThroughStore(context.TODO(), node)

	actors, err := state.GetActorsForMessage(context.TODO(), node, mid)
	if err != nil {
		return err
	}

	preTree, err := state.GetFilteredStateTree(context.TODO(), node, cache, mid, actors, true)
	if err != nil {
		return err
	}
	postTree, err := state.GetFilteredStateTree(context.TODO(), node, cache, mid, actors, false)
	if err != nil {
		return err
	}

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
			Epoch:     100,
			StateTree: &StateTree{
				CAR:     preData,
				RootCID: preRoot.String(),
			},
		},
		ApplyMessage: msgBytes,
		Post: &Postconditions{
			StateTree: &StateTree{
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
