package builders

import (
	"context"
	"log"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/account"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
	"github.com/ipfs/go-cid"
	cbg "github.com/whyrusleeping/cbor-gen"
)

type Actors struct {
	accounts []AddressHandle
	miners   []AddressHandle

	b *Builder
}

func (a *Actors) Accounts(typ address.Protocol, balance abi.TokenAmount, handles ...*AddressHandle) {
	for _, handle := range handles {
		h := a.Account(typ, balance)
		*handle = h
	}
}

func (a *Actors) Account(typ address.Protocol, balance abi.TokenAmount) AddressHandle {
	a.b.Assert.In(typ, address.SECP256K1, address.BLS)

	var addr address.Address
	switch typ {
	case address.SECP256K1:
		addr = a.b.Wallet.NewSECP256k1Account()
	case address.BLS:
		addr = a.b.Wallet.NewBLSAccount()
	}

	actorState := &account.State{Address: addr}
	handle := a.CreateActor(builtin.AccountActorCodeID, addr, balance, actorState)

	a.accounts = append(a.accounts, handle)
	return handle
}

type MinerActorCfg struct {
	SealProofType  abi.RegisteredSealProof
	PeriodBoundary abi.ChainEpoch
}

// create miner without sending assert message. modify the init and power actor manually
func (a *Actors) Miner(cfg MinerActorCfg) AddressHandle {
	owner := a.Account(address.SECP256K1, big.NewInt(1_000_000_000))
	worker := a.Account(address.BLS, big.Zero())
	// expectedMinerActorIDAddress := chain.MustNewIDAddr(chain.MustIDFromAddress(minerWorkerID) + 1)
	// minerActorAddrs := computeInitActorExecReturn(minerWorkerPk, 0, 1, expectedMinerActorIDAddress)

	ss, err := cfg.SealProofType.SectorSize()
	a.b.Assert.NoError(err, "seal proof sector size")

	ps, err := cfg.SealProofType.WindowPoStPartitionSectors()
	a.b.Assert.NoError(err, "seal proof window PoSt partition sectors")

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
	infoCid, err := a.b.cst.Put(context.Background(), mi)
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
	handle := a.CreateActor(builtin.StorageMinerActorCodeID, minerActorAddr, big.Zero(), minerState)

	// assert miner actor has been created, exists in the state tree, and has an entry in the init actor.
	// next update the storage power actor to track the miner

	var spa power.State
	a.ActorState(builtin.StoragePowerActorAddr, &spa)

	// set the miners claim
	hm, err := adt.AsMap(adt.WrapStore(context.Background(), a.b.cst), spa.Claims)
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
	_, err = a.b.cst.Put(context.Background(), &spa)
	if err != nil {
		panic(err)
	}

	a.miners = append(a.miners, handle)
	return handle
}

func (a *Actors) CreateActor(code cid.Cid, addr address.Address, balance abi.TokenAmount, actorState runtime.CBORMarshaler) AddressHandle {
	var id address.Address
	if addr.Protocol() != address.ID {
		var err error
		id, err = a.b.StateTree.RegisterNewAddress(addr)
		if err != nil {
			log.Panicf("register new address for actor: %v", err)
		}
	}

	// store the new state.
	head, err := a.b.StateTree.Store.Put(context.Background(), actorState)
	if err != nil {
		panic(err)
	}
	actr := &types.Actor{
		Code:    code,
		Head:    head,
		Balance: balance,
	}
	if err := a.b.StateTree.SetActor(addr, actr); err != nil {
		log.Panicf("setting new actor for actor: %v", err)
	}
	return AddressHandle{id, addr}
}

func (a *Actors) ActorState(addr address.Address, out cbg.CBORUnmarshaler) *types.Actor {
	actor, err := a.b.StateTree.GetActor(addr)
	a.b.Assert.NoError(err, "failed to fetch actor %s from state", addr)

	err = a.b.StateTree.Store.Get(context.Background(), actor.Head, out)
	a.b.Assert.NoError(err, "failed to load state for actorr %s; head=%s", addr, actor.Head)
	return actor
}

