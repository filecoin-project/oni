package drivers

import (
	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/stretchr/testify/require"

	"github.com/filecoin-project/oni/tvx/chain-validation/chain"
	"github.com/filecoin-project/oni/tvx/chain-validation/chain/types"
)

func NewTestDriver() *TestDriver {
	factory := NewFactories()
	syscalls := NewChainValidationSysCalls()
	stateWrapper, applier := factory.NewStateAndApplier(syscalls)
	sd := NewStateDriver(stateWrapper, factory.NewKeyManager())
	stateWrapper.NewVM()

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

	return &TestDriver{
		StateDriver:     sd,
		MessageProducer: producer,
		validator:       validator,
		ExeCtx:          exeCtx,

		Config: factory.NewValidationConfig(),

		SysCalls: syscalls,
	}
}
