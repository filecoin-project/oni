package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

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

	// Write out the test vector.
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
