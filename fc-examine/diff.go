package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/filecoin-project/lotus/api"
	"github.com/ipfs/go-cid"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/oni/fc-examine/lib"
	"github.com/filecoin-project/lotus/chain/types"
)

var diffFlags struct {
	selector string
}

var diffCmd = &cli.Command{
	Name:        "diff",
	Description: "Examine the state delta of an API object",
	Action:      runDiffCmd,
	Flags: []cli.Flag{
		&apiFlag,
	},
}

func objToStateTree(api api.FullNode, obj string, after bool) cid.Cid {
	objCid, err := cid.Parse(obj)
	if err != nil {
		// todo: log
		return cid.Undef
	}
	// see if obj is message
	if msg, err := api.StateSearchMsg(context.TODO(), objCid); err == nil {
		toCompute := []*types.Message{}
		if after {
			msgData, _ := api.ChainGetMessage(context.TODO(), objCid)
			toCompute = append(toCompute, msgData)
		}
		compute, err := api.StateCompute(context.TODO(), msg.Height, toCompute, msg.TipSet)
		if err != nil {
			// todo: log
			return cid.Undef
		}
		return compute.Root
	}
	// see if obj is block
	//ChainGetBlock(context.Context, cid.Cid) (*types.BlockHeader, error)
	// see if obj is tipset
	//ChainGetTipSet(context.Context, types.TipSetKey) (*types.TipSet, error)

	return cid.Undef
}

func runDiffCmd(c *cli.Context) error {
	if c.Args().Len() == 0 {
		return fmt.Errorf("no descriptor provided")
	}

	client, err := GetAPI(c)
	if err != nil {
		return err
	}

	store := lib.StoreFor(c.Context, client)

	var preCid, postCid cid.Cid

	obj := c.Args().Get(0)
	parts := strings.Split(obj, "..")

	if len(parts) == 1 {
		preCid = objToStateTree(client, parts[0], false)
		preCid = objToStateTree(client, parts[0], true)
	} else if len(parts) == 2{
		preCid = objToStateTree(client, parts[0], false)
		postCid = objToStateTree(client, parts[1], true)
	} else {
		return fmt.Errorf("invalid descriptor: %s", obj)
	}
	

	lib.Diff(
		c.Context,
		store,
		preCid,
		preCid,
		postCid)

	return nil
}
