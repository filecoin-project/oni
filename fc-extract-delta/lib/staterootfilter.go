package lib

import (
	"context"
	"fmt"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/state"
	cid "github.com/ipfs/go-cid"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/go-address"
)

// GetFilteredStateRoot provides the state tree needed to process a message represented as `msg`.
// The parent tipset on which the message was performed is loaded, and is then filtered to only
// include the actors referenced by the message.
func GetFilteredStateRoot(ctx context.Context, a api.FullNode, cache *CacheStore, msg cid.Cid, before bool) (*state.StateTree, error) {
	var tree *state.StateTree
	var err error
	msgInfo, err := a.StateSearchMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	if before {
		tree, err = getStateRootBeforeMsg(ctx, a, cache, msg, msgInfo.TipSet)
	} else {
		tree, err = getStateRootAfterMsg(ctx, a, cache, msg, msgInfo.TipSet)
	}
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

	return tree, nil
}

func getStateRootBeforeMsg(ctx context.Context, a api.FullNode, store *CacheStore, msg cid.Cid, tsk types.TipSetKey) (*state.StateTree, error) {
	ts, err := a.ChainGetTipSet(ctx, tsk)
	if err != nil {
		return nil, err
	}

	tree, err := state.LoadStateTree(store, ts.ParentState())
	if err != nil {
		return nil, err
	}

	return tree, nil
}

// GetStateRootAfterMsg is the complement to GetFilteredsStateRoot, returning the state root with
// msg applied.
func getStateRootAfterMsg(ctx context.Context, a api.FullNode, store *CacheStore, msg cid.Cid, tsk types.TipSetKey) (*state.StateTree, error) {
	ts, err := a.ChainGetTipSet(ctx, tsk)
	if err != nil {
		return nil, err
	}

	fullMsg, err := a.ChainGetMessage(ctx, msg)
	if err != nil {
		return nil, err
	}

	// TODO: this may need to be at the tsk's parent tipset
	postTree, err := a.StateCompute(ctx, ts.Height(), []*types.Message{fullMsg}, tsk)
	if err != nil {
		return nil, err
	}

	tree, err := state.LoadStateTree(store, postTree.Root)
	if err != nil {
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

	ts, err := a.ChainGetTipSet(ctx, msgInfo.TipSet)
	if err != nil {
		return nil, err
	}

	trace, err := a.StateReplay(ctx, ts.Parents(), msg)
	if err != nil {
		return nil, fmt.Errorf("could not replay msg: %w", err)
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
