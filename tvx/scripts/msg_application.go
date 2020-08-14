package main

import (
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	. "github.com/filecoin-project/oni/tvx/builders"
	"github.com/filecoin-project/oni/tvx/schema"
)

var (
	unknown      = MustNewIDAddr(10000000)
	aliceBal     = abi.NewTokenAmount(1_000_000_000_000)
	transferAmnt = abi.NewTokenAmount(10)
)

func main() {
	failCoverReceiptGasCost()
	failCoverOnChainSizeGasCost()
	failUnknownSender()
	failInvalidActorNonce()
	failInvalidReceiverMethod()
	failInexistentReceiver()
	failCoverAccountCreationGasStepwise()
}

func failCoverReceiptGasCost() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-receipt-gas",
		Version: "v1",
		Desc:    "fail to cover gas cost for message receipt on chain",
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	alice := v.Actors.Account(address.SECP256K1, aliceBal)
	v.CommitPreconditions()

	v.Messages.Sugar().Transfer(alice.ID, alice.ID, Value(transferAmnt), Nonce(0), GasLimit(8))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrOutOfGas))
	v.Finish(os.Stdout)
}

func failCoverOnChainSizeGasCost() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-onchainsize-gas",
		Version: "v1",
		Desc:    "not enough gas to pay message on-chain-size cost",
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(10))

	alice := v.Actors.Account(address.SECP256K1, aliceBal)
	v.CommitPreconditions()

	v.Messages.Sugar().Transfer(alice.ID, alice.ID, Value(transferAmnt), Nonce(0), GasLimit(1))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrOutOfGas))
	v.Finish(os.Stdout)
}

func failUnknownSender() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-unknown-sender",
		Version: "v1",
		Desc:    "fail due to lack of gas when sender is unknown",
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	alice := v.Actors.Account(address.SECP256K1, aliceBal)
	v.CommitPreconditions()

	v.Messages.Sugar().Transfer(unknown, alice.ID, Value(transferAmnt), Nonce(0))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrSenderInvalid))
	v.Finish(os.Stdout)
}

func failInvalidActorNonce() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-invalid-nonce",
		Version: "v1",
		Desc:    "invalid actor nonce",
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	alice := v.Actors.Account(address.SECP256K1, aliceBal)
	v.CommitPreconditions()

	// invalid nonce from known account.
	msg1 := v.Messages.Sugar().Transfer(alice.ID, alice.ID, Value(transferAmnt), Nonce(1))

	// invalid nonce from an unknown account.
	msg2 := v.Messages.Sugar().Transfer(unknown, alice.ID, Value(transferAmnt), Nonce(1))
	v.CommitApplies()

	v.Assert.Equal(msg1.Result.ExitCode, exitcode.SysErrSenderStateInvalid)
	v.Assert.Equal(msg2.Result.ExitCode, exitcode.SysErrSenderInvalid)

	v.Finish(os.Stdout)
}

func failInvalidReceiverMethod() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-invalid-receiver-method",
		Version: "v1",
		Desc:    "invalid receiver method",
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	alice := v.Actors.Account(address.SECP256K1, aliceBal)
	v.CommitPreconditions()

	v.Messages.Typed(alice.ID, alice.ID, MarketComputeDataCommitment(nil), Nonce(0), Value(big.Zero()))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrInvalidMethod))

	v.Finish(os.Stdout)
}

func failInexistentReceiver() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-inexistent-receiver",
		Version: "v1",
		Desc:    "inexistent receiver",
		Comment: `Note that this test is not a valid message, since it is using
an unknown actor. However in the event that an invalid message isn't filtered by
block validation we need to ensure behaviour is consistent across VM implementations.`,
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	alice := v.Actors.Account(address.SECP256K1, aliceBal)
	v.CommitPreconditions()

	// Sending a message to non-existent ID address must produce an error.
	unknownID := MustNewIDAddr(10000000)
	v.Messages.Sugar().Transfer(alice.ID, unknownID, Value(transferAmnt), Nonce(0))

	unknownActor := MustNewActorAddr("1234")
	v.Messages.Sugar().Transfer(alice.ID, unknownActor, Value(transferAmnt), Nonce(1))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrInvalidReceiver))
}

func failCoverAccountCreationGasStepwise() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-accountcreation-gas",
		Version: "v1",
		Desc:    "fail not enough gas to cover account actor creation",
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1))

	var alice, bob, charlie AddressHandle
	v.Actors.AccountN(address.SECP256K1, aliceBal, &alice)
	v.Actors.AccountN(address.SECP256K1, big.Zero(), &bob, &charlie)
	v.CommitPreconditions()

	var nonce uint64
	ref := v.Messages.Sugar().Transfer(alice.Robust, bob.Robust, Value(transferAmnt), Nonce(nonce))
	nonce++
	v.Messages.ApplyOne(ref)
	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.Ok))

	// decrease the gas cost by `gasStep` for each apply and ensure `SysErrOutOfGas` is always returned.
	trueGas := ref.Result.GasUsed
	gasStep := trueGas / 100
	for tryGas := trueGas - gasStep; tryGas > 0; tryGas -= gasStep {
		v.Messages.Sugar().Transfer(alice.Robust, charlie.Robust, Value(transferAmnt), Nonce(nonce), GasPrice(1), GasLimit(tryGas))
		nonce++
	}
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrOutOfGas), ref)
	v.Finish(os.Stdout)
}
