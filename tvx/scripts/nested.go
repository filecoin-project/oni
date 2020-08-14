package main

import (
	"bytes"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	builtin "github.com/filecoin-project/specs-actors/actors/builtin"
	init_ "github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/runtime"
	typegen "github.com/whyrusleeping/cbor-gen"
	//"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/builtin/multisig"
	//"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	. "github.com/filecoin-project/oni/tvx/builders"
	"github.com/filecoin-project/oni/tvx/schema"
	//"github.com/davecgh/go-spew/spew"
)

var (
	acctDefaultBalance = abi.NewTokenAmount(1_000_000_000_000)
	multisigBalance    = abi.NewTokenAmount(1_000_000_000)
	nonce              = uint64(1)
)

func main() {
	nestedSends_OkBasic()
	nestedSends_OkToNewActor()
	nestedSends_OkToNewActorWithInvoke()
	nestedSends_OkRecursive()
	nestedSends_OKNonCBORParamsWithTransfer()
}

func nestedSends_OkBasic() {
	metadata := &schema.Metadata{ID: "nested-sends-ok-basic", Version: "v1", Desc: ""}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	balanceBefore := v.Actors.Balance(stage.creator)

	// Multisig sends back to the creator.
	amtSent := abi.NewTokenAmount(1)
	result := stage.sendOk(stage.creator, amtSent, builtin.MethodSend, nil, nonce)

	//td.AssertActor(stage.creator, big.Sub(big.Add(balanceBefore, amtSent), result.Receipt.GasUsed.Big()), nonce+1)
	v.Assert.NonceEq(stage.creator, nonce+1)
	v.Assert.BalanceEq(stage.creator, big.Sub(big.Add(balanceBefore, amtSent), big.NewInt(result.MessageReceipt.GasUsed)))

	v.Finish(os.Stdout)
}

func nestedSends_OkToNewActor() {
	metadata := &schema.Metadata{ID: "nested-sends-ok-to-new-actor", Version: "v1", Desc: ""}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	balanceBefore := v.Actors.Balance(stage.creator)

	// Multisig sends to new address.
	newAddr := v.Wallet.NewSECP256k1Account()
	amtSent := abi.NewTokenAmount(1)
	result := stage.sendOk(newAddr, amtSent, builtin.MethodSend, nil, nonce)

	v.Assert.BalanceEq(stage.msAddr, big.Sub(multisigBalance, amtSent))
	v.Assert.BalanceEq(stage.creator, big.Sub(balanceBefore, big.NewInt(result.MessageReceipt.GasUsed)))
	v.Assert.BalanceEq(newAddr, amtSent)

	v.Finish(os.Stdout)
}

func nestedSends_OkToNewActorWithInvoke() {
	metadata := &schema.Metadata{ID: "nested-sends-ok-to-new-actor-with-invoke", Version: "v1", Desc: ""}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	balanceBefore := v.Actors.Balance(stage.creator)

	// Multisig sends to new address and invokes pubkey method at the same time.
	newAddr := v.Wallet.NewSECP256k1Account()
	amtSent := abi.NewTokenAmount(1)
	result := stage.sendOk(newAddr, amtSent, builtin.MethodsAccount.PubkeyAddress, nil, nonce)
	// TODO: use an explicit Approve() and check the return value is the correct pubkey address
	// when the multisig Approve() method plumbs through the inner exit code and value.
	// https://github.com/filecoin-project/specs-actors/issues/113
	//expected := bytes.Buffer{}
	//require.NoError(t, newAddr.MarshalCBOR(&expected))
	//assert.Equal(t, expected.Bytes(), result.Receipt.ReturnValue)

	v.Assert.BalanceEq(stage.msAddr, big.Sub(multisigBalance, amtSent))
	v.Assert.BalanceEq(stage.creator, big.Sub(balanceBefore, big.NewInt(result.MessageReceipt.GasUsed)))
	v.Assert.BalanceEq(newAddr, amtSent)

	v.Finish(os.Stdout)
}

func nestedSends_OkRecursive() {
	metadata := &schema.Metadata{ID: "nested-sends-ok-recursive", Version: "v1", Desc: ""}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	another := v.Actors.Account(address.SECP256K1, big.Zero())
	stage := prepareStage(v, acctDefaultBalance, multisigBalance)
	balanceBefore := v.Actors.Balance(stage.creator)

	// Multisig sends to itself.
	params := multisig.AddSignerParams{
		Signer:   another.ID,
		Increase: false,
	}
	result := stage.sendOk(stage.msAddr, big.Zero(), builtin.MethodsMultisig.AddSigner, &params, nonce)

	v.Assert.BalanceEq(stage.msAddr, multisigBalance)
	v.Assert.Equal(big.Sub(balanceBefore, big.NewInt(result.MessageReceipt.GasUsed)), v.Actors.Balance(stage.creator))

	var st multisig.State
	v.Actors.ActorState(stage.msAddr, &st)
	v.Assert.Equal([]address.Address{stage.creator, another.ID}, st.Signers)

	v.Finish(os.Stdout)
}

func nestedSends_OKNonCBORParamsWithTransfer() {
	metadata := &schema.Metadata{ID: "nested-sends-ok-non-cbor-params-with-transfer", Version: "v1", Desc: ""}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	stage := prepareStage(v, acctDefaultBalance, multisigBalance)

	newAddr := v.Wallet.NewSECP256k1Account()
	amtSent := abi.NewTokenAmount(1)
	// So long as the parameters are not actually used by the method, a message can carry arbitrary bytes.
	params := typegen.Deferred{Raw: []byte{1, 2, 3, 4}}
	stage.sendOk(newAddr, amtSent, builtin.MethodSend, &params, nonce)

	v.Assert.BalanceEq(stage.msAddr, big.Sub(multisigBalance, amtSent))
	v.Assert.BalanceEq(newAddr, amtSent)

	v.Finish(os.Stdout)
}

type msStage struct {
	v       *Builder
	creator address.Address // Address of the creator and sole signer of the multisig.
	msAddr  address.Address // Address of the multisig actor from which nested messages are sent.
}

// Creates a multisig actor with its creator as sole approver.
func prepareStage(v *Builder, creatorBalance, msBalance abi.TokenAmount) *msStage {
	// Set up sender and receiver accounts.
	creator := v.Actors.Account(address.SECP256K1, creatorBalance)
	v.CommitPreconditions()

	msg := v.Messages.Sugar().CreateMultisigActor(creator.ID, []address.Address{creator.ID}, 0, 1, Value(msBalance), Nonce(0))
	v.Messages.ApplyOne(msg)

	v.Assert.Equal(msg.Result.ExitCode, exitcode.Ok)

	// Verify init actor return.
	var ret init_.ExecReturn
	MustDeserialize(msg.Result.Return, &ret)

	return &msStage{
		v:       v,
		creator: creator.ID,
		msAddr:  ret.IDAddress,
	}
}

func (s *msStage) sendOk(to address.Address, value abi.TokenAmount, method abi.MethodNum, params runtime.CBORMarshaler, approverNonce uint64) *vm.ApplyRet {
	buf := bytes.Buffer{}
	if params != nil {
		err := params.MarshalCBOR(&buf)
		if err != nil {
			panic(err)
		}
	}
	pparams := multisig.ProposeParams{
		To:     to,
		Value:  value,
		Method: method,
		Params: buf.Bytes(),
	}
	msg := s.v.Messages.Typed(s.creator, s.msAddr, MultisigPropose(&pparams), Nonce(approverNonce), Value(big.NewInt(0)))
	s.v.CommitApplies()

	// all messages succeeded.
	s.v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.Ok))

	return msg.Result
}
