package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"

	"github.com/ipld/go-car"
	"github.com/minio/blake2b-simd"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-crypto"
	acrypto "github.com/filecoin-project/specs-actors/actors/crypto"

	"github.com/filecoin-project/lotus/chain/state"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/lib/blockstore"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	"github.com/ipfs/go-blockservice"
	"github.com/ipfs/go-datastore"
	offline "github.com/ipfs/go-ipfs-exchange-offline"
	cbor "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/puppet"
	"github.com/ipfs/go-cid"

	vtypes "github.com/filecoin-project/oni/tvx/chain-validation/chain/types"
	vstate "github.com/filecoin-project/oni/tvx/chain-validation/state"
	"github.com/filecoin-project/lotus/chain/stmgr"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
)

type Factories struct {
	*Applier
}

var _ vstate.Factories = &Factories{}

func NewFactories() *Factories {
	return &Factories{}
}

func (f *Factories) NewStateAndApplier(syscalls runtime.Syscalls) (vstate.VMWrapper, vstate.Applier) {
	st := NewState()
	return st, NewApplier(st, func(ctx context.Context, cstate *state.StateTree, cst cbor.IpldStore) runtime.Syscalls {
		return syscalls
	})
}

func (f *Factories) NewKeyManager() vstate.KeyManager {
	return newKeyManager()
}

func (f *Factories) NewValidationConfig() vstate.ValidationConfig {
	trackGas := true
	checkExit := true
	checkRet := true
	checkState := true
	return NewConfig(trackGas, checkExit, checkRet, checkState)
}

// Applier applies messages to state trees and storage.
type Applier struct {
	stateWrapper *StateWrapper
	syscalls     vm.SyscallBuilder
}

var _ vstate.Applier = &Applier{}

func NewApplier(sw *StateWrapper, syscalls vm.SyscallBuilder) *Applier {
	return &Applier{sw, syscalls}
}

func (a *Applier) ApplyMessage(epoch abi.ChainEpoch, message *vtypes.Message) (vtypes.ApplyMessageResult, error) {
	lm := toLotusMsg(message)
	receipt, penalty, reward, err := a.applyMessage(epoch, lm)

	var buffer bytes.Buffer

	serialise(a.stateWrapper.bs, a.stateWrapper.Root(), &buffer)

	hexenc := hex.EncodeToString(buffer.Bytes())
	fmt.Println(hexenc)

	return vtypes.ApplyMessageResult{
		Receipt: receipt,
		Penalty: penalty,
		Reward:  reward,
		Root:    a.stateWrapper.Root().String(),
	}, err
}

func (a *Applier) ApplySignedMessage(epoch abi.ChainEpoch, msg *vtypes.SignedMessage) (vtypes.ApplyMessageResult, error) {
	var lm types.ChainMsg
	switch msg.Signature.Type {
	case acrypto.SigTypeSecp256k1:
		lm = toLotusSignedMsg(msg)
	case acrypto.SigTypeBLS:
		lm = toLotusMsg(&msg.Message)
	default:
		return vtypes.ApplyMessageResult{}, errors.New("Unknown signature type")
	}
	// TODO: Validate the sig first
	receipt, penalty, reward, err := a.applyMessage(epoch, lm)
	return vtypes.ApplyMessageResult{
		Receipt: receipt,
		Penalty: penalty,
		Reward:  reward,
		Root:    a.stateWrapper.Root().String(),
	}, err

}

func (a *Applier) ApplyTipSetMessages(epoch abi.ChainEpoch, blocks []vtypes.BlockMessagesInfo, rnd vstate.RandomnessSource) (vtypes.ApplyTipSetResult, error) {
	cs := store.NewChainStore(a.stateWrapper.bs, a.stateWrapper.ds, a.syscalls)
	sm := stmgr.NewStateManager(cs)

	var bms []stmgr.BlockMessages
	for _, b := range blocks {
		bm := stmgr.BlockMessages{
			Miner:    b.Miner,
			WinCount: 1,
		}

		for _, m := range b.BLSMessages {
			bm.BlsMessages = append(bm.BlsMessages, toLotusMsg(m))
		}

		for _, m := range b.SECPMessages {
			bm.SecpkMessages = append(bm.SecpkMessages, toLotusSignedMsg(m))
		}

		bms = append(bms, bm)
	}

	var receipts []vtypes.MessageReceipt
	sroot, _, err := sm.ApplyBlocks(context.TODO(), epoch-1, a.stateWrapper.Root(), bms, epoch, &randWrapper{rnd}, func(c cid.Cid, msg *types.Message, ret *vm.ApplyRet) error {
		if msg.From == builtin.SystemActorAddr {
			return nil // ignore reward and cron calls
		}
		rval := ret.Return
		if rval == nil {
			rval = []byte{} // chain validation tests expect empty arrays to not be nil...
		}
		receipts = append(receipts, vtypes.MessageReceipt{
			ExitCode:    ret.ExitCode,
			ReturnValue: rval,

			GasUsed: vtypes.GasUnits(ret.GasUsed),
		})
		return nil
	})
	if err != nil {
		return vtypes.ApplyTipSetResult{}, err
	}

	a.stateWrapper.stateRoot = sroot

	return vtypes.ApplyTipSetResult{
		Receipts: receipts,
		Root:     a.stateWrapper.Root().String(),
	}, nil
}

type randWrapper struct {
	rnd vstate.RandomnessSource
}

func (w *randWrapper) GetRandomness(ctx context.Context, pers acrypto.DomainSeparationTag, round abi.ChainEpoch, entropy []byte) ([]byte, error) {
	return w.rnd.Randomness(ctx, pers, round, entropy)
}

//type vmRand struct {
//}

//func (*vmRand) GetRandomness(ctx context.Context, dst crypto.DomainSeparationTag, h abi.ChainEpoch, input []byte) ([]byte, error) {
//panic("implement me")
//}

func (a *Applier) applyMessage(epoch abi.ChainEpoch, lm types.ChainMsg) (vtypes.MessageReceipt, abi.TokenAmount, abi.TokenAmount, error) {
	ctx := context.TODO()
	base := a.stateWrapper.Root()

	lotusVM, err := vm.NewVM(base, epoch, &vmRand{}, a.stateWrapper.bs, a.syscalls, nil)
	// need to modify the VM invoker to add the puppet actor
	chainValInvoker := vm.NewInvoker()
	chainValInvoker.Register(puppet.PuppetActorCodeID, puppet.Actor{}, puppet.State{})
	lotusVM.SetInvoker(chainValInvoker)
	if err != nil {
		return vtypes.MessageReceipt{}, big.Zero(), big.Zero(), err
	}

	ret, err := lotusVM.ApplyMessage(ctx, lm)
	if err != nil {
		return vtypes.MessageReceipt{}, big.Zero(), big.Zero(), err
	}

	rval := ret.Return
	if rval == nil {
		rval = []byte{}
	}

	a.stateWrapper.stateRoot, err = lotusVM.Flush(ctx)
	if err != nil {
		return vtypes.MessageReceipt{}, big.Zero(), big.Zero(), err
	}

	mr := vtypes.MessageReceipt{
		ExitCode:    ret.ExitCode,
		ReturnValue: rval,
		GasUsed:     vtypes.GasUnits(ret.GasUsed),
	}

	return mr, ret.Penalty, abi.NewTokenAmount(ret.GasUsed), nil
}

func toLotusMsg(msg *vtypes.Message) *types.Message {
	return &types.Message{
		To:   msg.To,
		From: msg.From,

		Nonce:  msg.CallSeqNum,
		Method: msg.Method,

		Value:    types.BigInt{Int: msg.Value.Int},
		GasPrice: types.BigInt{Int: msg.GasPrice.Int},
		GasLimit: msg.GasLimit,

		Params: msg.Params,
	}
}

func toLotusSignedMsg(msg *vtypes.SignedMessage) *types.SignedMessage {
	return &types.SignedMessage{
		Message:   *toLotusMsg(&msg.Message),
		Signature: msg.Signature,
	}
}

var _ vstate.VMWrapper = &StateWrapper{}

type StateWrapper struct {
	// The blockstore underlying the state tree and storage.
	bs blockstore.Blockstore

	ds datastore.Batching
	// HAMT-CBOR store on top of the blockstore.
	cst cbor.IpldStore

	// CID of the root of the state tree.
	stateRoot cid.Cid
}

func NewState() *StateWrapper {
	bs := blockstore.NewTemporary()
	cst := cbor.NewCborStore(bs)
	// Put EmptyObjectCid value in the store. When an actor is initially created its Head is set to this value.
	_, err := cst.Put(context.TODO(), map[string]string{})
	if err != nil {
		panic(err)
	}

	treeImpl, err := state.NewStateTree(cst)
	if err != nil {
		panic(err) // Never returns error, the error return should be removed.
	}
	root, err := treeImpl.Flush(context.TODO())
	if err != nil {
		panic(err)
	}
	return &StateWrapper{
		bs:        bs,
		ds:        datastore.NewMapDatastore(),
		cst:       cst,
		stateRoot: root,
	}
}

func (s *StateWrapper) NewVM() {
	return
}

func (s *StateWrapper) Root() cid.Cid {
	return s.stateRoot
}

// StoreGet the value at key from vm store
func (s *StateWrapper) StoreGet(key cid.Cid, out runtime.CBORUnmarshaler) error {
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return err
	}
	return tree.Store.Get(context.Background(), key, out)
}

// StorePut `value` into vm store
func (s *StateWrapper) StorePut(value runtime.CBORMarshaler) (cid.Cid, error) {
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return cid.Undef, err
	}
	return tree.Store.Put(context.Background(), value)
}

func (s *StateWrapper) Actor(addr address.Address) (vstate.Actor, error) {
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, err
	}
	fcActor, err := tree.GetActor(addr)
	if err != nil {
		return nil, err
	}
	return &actorWrapper{*fcActor}, nil
}

func (s *StateWrapper) SetActorState(addr address.Address, balance abi.TokenAmount, actorState runtime.CBORMarshaler) (vstate.Actor, error) {
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, err
	}
	// actor should exist
	act, err := tree.GetActor(addr)
	if err != nil {
		return nil, err
	}
	// add the state to the store and get a new head cid
	actHead, err := tree.Store.Put(context.Background(), actorState)
	if err != nil {
		return nil, err
	}
	// update the actor object with new head and balance parameter
	actr := &actorWrapper{types.Actor{
		Code:  act.Code,
		Nonce: act.Nonce,
		// updates
		Head:    actHead,
		Balance: balance,
	}}
	if err := tree.SetActor(addr, &actr.Actor); err != nil {
		return nil, err
	}
	return actr, s.flush(tree)
}

func (s *StateWrapper) CreateActor(code cid.Cid, addr address.Address, balance abi.TokenAmount, actorState runtime.CBORMarshaler) (vstate.Actor, address.Address, error) {
	idAddr := addr
	tree, err := state.LoadStateTree(s.cst, s.stateRoot)
	if err != nil {
		return nil, address.Undef, err
	}
	if addr.Protocol() != address.ID {

		actHead, err := tree.Store.Put(context.Background(), actorState)
		if err != nil {
			return nil, address.Undef, err
		}
		actr := &actorWrapper{types.Actor{
			Code:    code,
			Head:    actHead,
			Balance: balance,
		}}

		idAddr, err = tree.RegisterNewAddress(addr)
		if err != nil {
			return nil, address.Undef, fmt.Errorf("register new address for actor: %w", err)
		}

		if err := tree.SetActor(addr, &actr.Actor); err != nil {
			return nil, address.Undef, fmt.Errorf("setting new actor for actor: %w", err)
		}
	}

	// store newState
	head, err := tree.Store.Put(context.Background(), actorState)
	if err != nil {
		return nil, address.Undef, err
	}

	// create and store actor object
	a := types.Actor{
		Code:    code,
		Head:    head,
		Balance: balance,
	}
	if err := tree.SetActor(idAddr, &a); err != nil {
		return nil, address.Undef, err
	}

	return &actorWrapper{a}, idAddr, s.flush(tree)
}

// Flushes a state tree to storage and sets this state's root to that tree's root CID.
func (s *StateWrapper) flush(tree *state.StateTree) (err error) {
	s.stateRoot, err = tree.Flush(context.TODO())
	return
}

//
// Actor Wrapper
//

type actorWrapper struct {
	types.Actor
}

func (a *actorWrapper) Code() cid.Cid {
	return a.Actor.Code
}

func (a *actorWrapper) Head() cid.Cid {
	return a.Actor.Head
}

func (a *actorWrapper) CallSeqNum() uint64 {
	return a.Actor.Nonce
}

func (a *actorWrapper) Balance() big.Int {
	return a.Actor.Balance

}

//
// Storage
//

type contextStore struct {
	cbor.IpldStore
	ctx context.Context
}

func (s *contextStore) Context() context.Context {
	return s.ctx
}

type KeyManager struct {
	// Private keys by address
	keys map[address.Address]*wallet.Key

	// Seed for deterministic secp key generation.
	secpSeed int64
	// Seed for deterministic bls key generation.
	blsSeed int64 // nolint: structcheck
}

func newKeyManager() *KeyManager {
	return &KeyManager{
		keys:     make(map[address.Address]*wallet.Key),
		secpSeed: 0,
	}
}

func (k *KeyManager) NewSECP256k1AccountAddress() address.Address {
	secpKey := k.newSecp256k1Key()
	k.keys[secpKey.Address] = secpKey
	return secpKey.Address
}

func (k *KeyManager) NewBLSAccountAddress() address.Address {
	blsKey := k.newBLSKey()
	k.keys[blsKey.Address] = blsKey
	return blsKey.Address
}

func (k *KeyManager) Sign(addr address.Address, data []byte) (acrypto.Signature, error) {
	ki, ok := k.keys[addr]
	if !ok {
		return acrypto.Signature{}, fmt.Errorf("unknown address %v", addr)
	}
	var sigType acrypto.SigType
	if ki.Type == wallet.KTSecp256k1 {
		sigType = acrypto.SigTypeBLS
		hashed := blake2b.Sum256(data)
		sig, err := crypto.Sign(ki.PrivateKey, hashed[:])
		if err != nil {
			return acrypto.Signature{}, err
		}

		return acrypto.Signature{
			Type: sigType,
			Data: sig,
		}, nil
	} else if ki.Type == wallet.KTBLS {
		panic("lotus validator cannot sign BLS messages")
	} else {
		panic("unknown signature type")
	}

}

func (k *KeyManager) newSecp256k1Key() *wallet.Key {
	randSrc := rand.New(rand.NewSource(k.secpSeed))
	prv, err := crypto.GenerateKeyFromSeed(randSrc)
	if err != nil {
		panic(err)
	}
	k.secpSeed++
	key, err := wallet.NewKey(types.KeyInfo{
		Type:       wallet.KTSecp256k1,
		PrivateKey: prv,
	})
	if err != nil {
		panic(err)
	}
	return key
}

func (k *KeyManager) newBLSKey() *wallet.Key {
	// FIXME: bls needs deterministic key generation
	//sk := ffi.PrivateKeyGenerate(s.blsSeed)
	// s.blsSeed++
	sk := [32]byte{}
	sk[0] = uint8(k.blsSeed) // hack to keep gas values determinist
	k.blsSeed++
	key, err := wallet.NewKey(types.KeyInfo{
		Type:       wallet.KTBLS,
		PrivateKey: sk[:],
	})
	if err != nil {
		panic(err)
	}
	return key
}

//
// Config
//

type Config struct {
	trackGas         bool
	checkExitCode    bool
	checkReturnValue bool
	checkState       bool
}

func NewConfig(gas, exit, ret, state bool) *Config {
	return &Config{
		trackGas:         gas,
		checkExitCode:    exit,
		checkReturnValue: ret,
		checkState:       state,
	}
}

func (v Config) ValidateGas() bool {
	return v.trackGas
}

func (v Config) ValidateExitCode() bool {
	return v.checkExitCode
}

func (v Config) ValidateReturnValue() bool {
	return v.checkReturnValue
}

func (v Config) ValidateStateRoot() bool {
	return v.checkState
}

func serialise(bs blockstore.Blockstore, c cid.Cid, w io.Writer) error {
	ctx := context.Background()

	offl := offline.Exchange(bs)
	blkserv := blockservice.New(bs, offl)
	dserv := merkledag.NewDAGService(blkserv)

	if err := car.WriteCarWithWalker(ctx, dserv, []cid.Cid{c}, w, walker); err != nil {
		return fmt.Errorf("failed to write car file: %w", err)
	}

	return nil
}

func walker(nd format.Node) (out []*format.Link, err error) {
	for _, link := range nd.Links() {
		//spew.Dump(link)
		if link.Cid.Prefix().Codec == cid.FilCommitmentSealed || link.Cid.Prefix().Codec == cid.FilCommitmentUnsealed {
			continue
		}
		out = append(out, link)
	}

	return out, nil
}
