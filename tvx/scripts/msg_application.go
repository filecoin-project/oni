package main

import (
	"os"

	"github.com/filecoin-project/go-address"
	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	. "github.com/filecoin-project/oni/tvx/builders"
	"github.com/filecoin-project/oni/tvx/schema"
)

var (
	aliceBal     = abi_spec.NewTokenAmount(1_000_000_000_000)
	transferAmnt = abi_spec.NewTokenAmount(10)
)

func main() {
	failCoverReceiptGasCost()
}

func failCoverReceiptGasCost() {
	metadata := &schema.Metadata{
		ID:      "msg-apply-fail-receipt-gas",
		Version: "v1",
		Desc:    "fail to cover gas cost for message receipt on chain",
	}

	v := MessageVector(metadata)
	v.Messages.SetDefaults(GasLimit(1_000_000_000), GasPrice(1), GasLimit(8))

	alice := v.Actors.Account(address.SECP256K1, aliceBal)
	v.CommitPreconditions()

	v.Messages.Sugar().Transfer(alice.ID, alice.ID, Value(transferAmnt), Nonce(0))
	v.CommitApplies()

	v.Assert.EveryMessageResultSatisfies(ExitCode(exitcode.SysErrOutOfGas))
	v.Finish(os.Stdout)
}
