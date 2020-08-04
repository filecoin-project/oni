package main

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"

	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	exitcode_spec "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	addr "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/oni/tvx/chain-validation/chain"
	"github.com/filecoin-project/oni/tvx/chain-validation/drivers"
)

var messagesTestCmd = &cli.Command{
	Name:        "messages-test",
	Description: "",
	Flags:       []cli.Flag{&cidFlag, &apiFlag},
	Action:      runMessagesTest,
}

func runMessagesTest(c *cli.Context) error {
	// input / output params
	existingActorType := address.SECP256K1
	existingActorBal := abi_spec.NewTokenAmount(10000000000)

	newActorAddr, err := addr.NewSecp256k1Address([]byte("publickeyfoo"))
	if err != nil {
		panic(err)
	}
	newActorInitBal := abi_spec.NewTokenAmount(10000)

	expExitCode := exitcode_spec.Ok

	//factory := NewFactories()
	//builder := drivers.NewBuilder(context.Background(), factory).WithDefaultGasLimit(1000000000).WithDefaultGasPrice(big_spec.NewInt(1)).WithActorState(drivers.DefaultBuiltinActorsState...)

	td := drivers.NewTestDriver()

	existingAccountAddr, _ := td.NewAccountActor(existingActorType, existingActorBal)
	msg := td.MessageProducer.Transfer(existingAccountAddr, newActorAddr, chain.Value(newActorInitBal), chain.Nonce(0))
	spew.Dump(msg)
	result := td.ApplyOk(
		msg,
		//expExitCode,
	)
	spew.Dump(result)

	// new actor balance will only exist if message was applied successfully.
	if expExitCode.IsSuccess() {
		td.AssertBalance(newActorAddr, newActorInitBal)
		td.AssertBalance(existingAccountAddr, big_spec.Sub(big_spec.Sub(existingActorBal, result.Receipt.GasUsed.Big()), newActorInitBal))
	}
	return nil
}
