package lib

import (
	"context"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/state"
	cid "github.com/ipfs/go-cid"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/go-address"
)

// GetFilteredStateRoot provides the state tree needed to process a message represented as `msg`.
// The parent tipset on which the message was performed is loaded, and is then filtered to only
// include the actors referenced by the message.
func GetFilteredStateRoot(ctx context.Context, a api.FullNode, msg cid.Cid) (*state.StateTree, error) {
	msgInfo, err := a.StateSearchMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	ts, err := a.ChainGetTipSet(ctx, msgInfo.TipSet)
	if err != nil {
		return nil, err
	}

	store := apiIpldStore{ctx, a}
	tree, err := state.LoadStateTree(&store, ts.ParentState())
	if err != nil {
		return nil, err
	}

	goodActors, err := GetActorsForMessage(ctx, a, msg)
	if err != nil {
		return nil, err
	}

	allActors, err := a.StateListActors(ctx, msgInfo.TipSet)
	if err != nil {
		return nil, err
	}

	for _, act := range allActors {
		if _, ok := goodActors[act]; ok {
			continue
		}

		if err := tree.DeleteActor(act); err != nil {
			return nil, err
		}
	}

	if 	_, err = tree.Flush(ctx); err != nil {
		return nil, err
	}

	return tree, nil
}

// GetActorsForMessage queries a message by Cid to return the array of actors taht are referenced by it.
func GetActorsForMessage(ctx context.Context, a api.FullNode, msg cid.Cid) (map[address.Address]struct{}, error) {
	msgInfo, err := a.StateSearchMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	trace, err := a.StateReplay(ctx, msgInfo.TipSet, msg)
	if err != nil {
		return nil, err
	}

	addresses := make(map[address.Address]struct{})
	populateFromTrace(&addresses, &trace.ExecutionTrace)
	return addresses, nil
}

func populateFromTrace(m *map[address.Address]struct{}, trace *types.ExecutionTrace) {
	(*m)[trace.Msg.To] = struct{}{}
	(*m)[trace.Msg.From] = struct{}{}

	for _, s := range trace.Subcalls {
		populateFromTrace(m, &s)
	}
}
