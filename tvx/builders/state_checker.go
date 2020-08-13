package builders

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
	cbg "github.com/whyrusleeping/cbor-gen"
)

type StateChecker struct {
	a  *Asserter
	st *state.StateTree
}

func (c *StateChecker) BalanceEqual(addr address.Address, expected abi.TokenAmount) {
	actor, err := c.st.GetActor(addr)
	c.a.NoError(err, "failed to fetch actor %s from state", addr)
	c.a.Equal(expected, actor.Balance, "balances mismatch for address %s", addr)
}

func (c *StateChecker) LoadActorState(addr address.Address, out cbg.CBORUnmarshaler) *types.Actor {
	actor, err := c.st.GetActor(addr)
	c.a.NoError(err, "failed to fetch actor %s from state", addr)

	err = c.st.Store.Get(context.Background(), actor.Head, out)
	c.a.NoError(err, "failed to load state for actorr %s; head=%s", addr, actor.Head)
	return actor
}
