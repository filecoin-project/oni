package state

import (
	"bytes"
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
)

type Surgeon struct {
	ctx    context.Context
	api    api.FullNode
	stores *ProxyingStores
}

// NewSurgeon returns a state surgeon, an object used to fetch and manipulate state.
func NewSurgeon(ctx context.Context, api api.FullNode, stores *ProxyingStores) *Surgeon {
	return &Surgeon{
		ctx:    ctx,
		api:    api,
		stores: stores,
	}
}

// GetMaskedStateTree provides the state tree needed to process a message represented as `msg`.
// The parent tipset on which the message was performed is loaded, and is then filtered to only
// include the actors referenced by the message.
func (sg *Surgeon) GetMaskedStateTree(tsk types.TipSetKey, retain map[address.Address]struct{}) (*state.StateTree, cid.Cid, error) {
	// stateMap
	bs := adt.WrapStore(sg.ctx, sg.stores.CBORStore)

	stateMap := adt.MakeEmptyMap(bs)

	// LOAD THE INIT ACTOR ----------------
	initActor, err := sg.api.StateGetActor(sg.ctx, builtin.InitActorAddr, tsk)
	if err != nil {
		return nil, cid.Undef, err
	}

	initBytes, err := sg.api.ChainReadObj(sg.ctx, initActor.Head)
	if err != nil {
		return nil, cid.Undef, err
	}

	var ist init_.State
	err = ist.UnmarshalCBOR(bytes.NewReader(initBytes))
	if err != nil {
		return nil, cid.Undef, err
	}

	initMap, err := adt.AsMap(bs, ist.AddressMap)
	if err != nil {
		return nil, cid.Undef, err
	}

	// RESOLVE ADDRESSES ----------------
	finalAddresses := make(map[address.Address]struct{}, len(retain))
	for r := range retain {
		resolved, err := ist.ResolveAddress(bs, r)
		if err != nil {
			return nil, cid.Undef, err
		}

		finalAddresses[resolved] = struct{}{}
	}

	initActorAMT := adt.MakeEmptyMap(bs)
	for r := range retain {
		if r.Protocol() == address.ID {
			// skip over ID addresses; they don't need a mapping in the init actor.
			continue
		}

		var d cbg.Deferred
		if _, err := initMap.Get(adt.AddrKey(r), &d); err != nil {
			return nil, cid.Undef, err
		}
		if err := initActorAMT.Put(adt.AddrKey(r), &d); err != nil {
			return nil, cid.Undef, err
		}
	}

	// ---------- SAVE THE INIT ACTOR.
	rootCid, err := initActorAMT.Root()
	if err != nil {
		return nil, cid.Undef, err
	}

	s := &init_.State{
		NetworkName: ist.NetworkName,
		NextID:      ist.NextID,
		AddressMap:  rootCid,
	}

	statecid, err := bs.Put(sg.ctx, s)
	if err != nil {
		return nil, cid.Undef, err
	}

	act := &types.Actor{
		Code: builtin.InitActorCodeID,
		Head: statecid,
	}

	err = stateMap.Put(adt.AddrKey(builtin.InitActorAddr), act)
	if err != nil {
		return nil, cid.Undef, err
	}

	// -------- PLUCK ACTOR STATE.
	for a := range finalAddresses {
		actor, err := sg.api.StateGetActor(sg.ctx, a, tsk)
		if err != nil {
			return nil, cid.Undef, err
		}

		err = stateMap.Put(adt.AddrKey(a), actor)
		if err != nil {
			return nil, cid.Undef, err
		}

		// recursive copy of the actor state so we can
		err = vm.Copy(sg.stores.Blockstore, sg.stores.Blockstore, actor.Head)
		if err != nil {
			return nil, cid.Undef, err
		}

		actorState, err := sg.api.ChainReadObj(sg.ctx, actor.Head)
		if err != nil {
			return nil, cid.Undef, err
		}

		objCid, err := bs.Put(sg.ctx, &cbg.Deferred{Raw: actorState})
		if err != nil {
			return nil, cid.Undef, err
		}

		if objCid != actor.Head {
			panic("mismatched CIDs")
		}
	}

	stateRoot, err := stateMap.Root()
	if err != nil {
		return nil, cid.Undef, err
	}

	tree, err := state.LoadStateTree(bs, stateRoot)
	if err != nil {
		return nil, cid.Undef, err
	}

	keys, err := stateMap.CollectKeys()
	if err != nil {
		return nil, cid.Undef, err
	}

	for _, a := range keys {
		fmt.Println(address.NewFromBytes([]byte(a)))
	}

	return tree, stateRoot, nil
}

// GetAccessedActors identifies the actors that were accessed during the
// execution of a message.
func (sg *Surgeon) GetAccessedActors(ctx context.Context, a api.FullNode, msg cid.Cid) (map[address.Address]struct{}, error) {
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
	populateFromTrace(addresses, &trace.ExecutionTrace)
	return addresses, nil
}

func populateFromTrace(m map[address.Address]struct{}, trace *types.ExecutionTrace) {
	m[trace.Msg.To] = struct{}{}
	m[trace.Msg.From] = struct{}{}

	for _, s := range trace.Subcalls {
		populateFromTrace(m, &s)
	}
}
