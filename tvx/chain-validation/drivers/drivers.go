package drivers

import (
	"context"

	"github.com/filecoin-project/lotus/chain/state"
	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/oni/tvx/chain-validation/chain"
	"github.com/filecoin-project/oni/tvx/chain-validation/chain/types"
)

func NewTestDriver() *TestDriver {
	syscalls := NewChainValidationSysCalls()
	stateWrapper := NewState()
	applier := NewApplier(stateWrapper, func(ctx context.Context, cstate *state.StateTree, cst cbor.IpldStore) runtime.Syscalls {
		return syscalls
	})

	sd := NewStateDriver(stateWrapper, newKeyManager())

	err := initializeStoreWithAdtRoots(AsStore(sd.st))
	require.NoError(t, err)

	for _, acts := range DefaultBuiltinActorsState {
		_, _, err := sd.State().CreateActor(acts.Code, acts.Addr, acts.Balance, acts.State)
		require.NoError(t, err)
	}

	minerActorIDAddr := sd.newMinerAccountActor(TestSealProofType, abi_spec.ChainEpoch(0))

	exeCtx := types.NewExecutionContext(1, minerActorIDAddr)
	producer := chain.NewMessageProducer(1000000000, big_spec.NewInt(1)) // gas limit ; gas price
	validator := chain.NewValidator(applier)

	trackGas := false
	checkExit := true
	checkRet := true
	checkState := true
	config := NewConfig(trackGas, checkExit, checkRet, checkState)

	return &TestDriver{
		StateDriver:     sd,
		MessageProducer: producer,
		validator:       validator,
		ExeCtx:          exeCtx,
		Config:          config,
		SysCalls:        syscalls,
	}
}
