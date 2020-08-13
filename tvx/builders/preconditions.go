package builders

import (
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

type Preconditions struct{
	assert *Asserter
	b      *Builder
}

func (p *Preconditions) AccountActors(typ address.Protocol, balance abi.TokenAmount, handles... *AddressHandle) {
	for _, handle := range handles {
		h := p.AccountActor(typ, balance)
		*handle = h
	}
}

func (p *Preconditions) AccountActor(typ address.Protocol, balance abi.TokenAmount) AddressHandle {
	p.assert.In(typ, address.SECP256K1, address.BLS)

	var addr address.Address
	switch typ {
	case address.SECP256K1:
		addr = p.b.Wallet.NewSECP256k1Account()
	case address.BLS:
		addr = p.b.Wallet.NewBLSAccount()
	}

	actorState := &account.State{Address: addr}
	handle := p.b.CreateActor(builtin.AccountActorCodeID, addr, balance, actorState)

	p.b.AccountActors = append(p.b.AccountActors, handle)
	return handle
}

type MinerActorCfg struct {
	SealProofType  abi.RegisteredSealProof
	PeriodBoundary abi.ChainEpoch
}

// create miner without sending assert message. modify the init and power actor manually
func (p *Preconditions) MinerActor(cfg MinerActorCfg) AddressHandle {
	owner := p.AccountActor(address.SECP256K1, big.NewInt(1_000_000_000))
	worker := p.AccountActor(address.BLS, big.Zero())
	// expectedMinerActorIDAddress := chain.MustNewIDAddr(chain.MustIDFromAddress(minerWorkerID) + 1)
	// minerActorAddrs := computeInitActorExecReturn(minerWorkerPk, 0, 1, expectedMinerActorIDAddress)

	ss, err := cfg.SealProofType.SectorSize()
	p.assert.NoError(err, "seal proof sector size")

	ps, err := cfg.SealProofType.WindowPoStPartitionSectors()
	p.assert.NoError(err, "seal proof window PoSt partition sectors")

	mi := &miner.MinerInfo{
		Owner:                      owner.ID,
		Worker:                     worker.ID,
		PendingWorkerKey:           nil,
		PeerId:                     abi.PeerID("chain-validation"),
		Multiaddrs:                 nil,
		SealProofType:              cfg.SealProofType,
		SectorSize:                 ss,
		WindowPoStPartitionSectors: ps,
	}
	infoCid, err := p.b.cst.Put(context.Background(), mi)
	if err != nil {
		panic(err)
	}

	// create the miner actor s.t. it exists in the init actors map
	minerState, err := miner.ConstructState(infoCid,
		cfg.PeriodBoundary,
		EmptyBitfieldCid,
		EmptyArrayCid,
		EmptyMapCid,
		EmptyDeadlinesCid,
	)
	if err != nil {
		panic(err)
	}

	minerActorAddr := worker.NextActorAddress(0, 0)
	handle := p.b.CreateActor(builtin.StorageMinerActorCodeID, minerActorAddr, big.Zero(), minerState)

	// assert miner actor has been created, exists in the state tree, and has an entry in the init actor.
	// next update the storage power actor to track the miner

	var spa power.State
	p.b.GetActorState(builtin.StoragePowerActorAddr, &spa)

	// set the miners claim
	hm, err := adt.AsMap(adt.WrapStore(context.Background(), p.b.cst), spa.Claims)
	if err != nil {
		panic(err)
	}

	// add claim for the miner
	err = hm.Put(adt.AddrKey(handle.ID), &power.Claim{
		RawBytePower:    abi.NewStoragePower(0),
		QualityAdjPower: abi.NewTokenAmount(0),
	})
	if err != nil {
		panic(err)
	}

	// save the claim
	spa.Claims, err = hm.Root()
	if err != nil {
		panic(err)
	}

	// update miner count
	spa.MinerCount += 1

	// update storage power actor's state in the tree
	_, err = p.b.cst.Put(context.Background(), &spa)
	if err != nil {
		panic(err)
	}

	p.b.Miners = append(p.b.Miners, handle)
	return handle
}
