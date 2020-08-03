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

	preData, err := state.SerializeStateTree(context.TODO(), preTree)
	if err != nil {
		return err
	}
	postData, err := state.SerializeStateTree(context.TODO(), postTree)
	if err != nil {
		return err
	}

	version, err := node.Version(context.TODO())
	if err != nil {
		return err
	}

	preObj := HexEncodedBytes(preData)
	postObj := HexEncodedBytes(postData)
	vector := TestVector{
		Class:    ClassMessages,
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
			StateTree: &preObj,
		},
		ApplyMessages: []HexEncodedBytes{
			msgBytes,
		},
		Post: &Postconditions{
			StateTree: &postObj,
		},
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&vector); err != nil {
		return err
	}

	return nil
}
