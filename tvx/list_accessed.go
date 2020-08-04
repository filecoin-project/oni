package main

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/oni/tvx/state"
)

var listAccessedCmd = &cli.Command{
	Name:        "list-accessed",
	Description: "extract actors accessed during the execution of a message",
	Flags:       []cli.Flag{&cidFlag, &apiFlag},
	Action:      runListAccessed,
}

func runListAccessed(c *cli.Context) error {
	ctx := context.Background()

	node, err := makeClient(c.String(apiFlag.Name))
	if err != nil {
		return err
	}

	mid, err := cid.Decode(c.String(cidFlag.Name))
	if err != nil {
		return err
	}

	rtst := state.NewProxyingStore(ctx, node)

	g := state.NewSurgeon(ctx, node, rtst)

	actors, err := g.GetAccessedActors(context.TODO(), node, mid)
	if err != nil {
		return err
	}

	for k := range actors {
		fmt.Printf("%v\n", k)
	}
	return nil
}
