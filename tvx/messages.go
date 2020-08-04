package main

import (
	"encoding/json"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"

	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	exitcode_spec "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	addr "github.com/filecoin-project/go-address"
	"github.com/filecoin-project/oni/tvx/chain"
	"github.com/filecoin-project/oni/tvx/drivers"
	"github.com/filecoin-project/oni/tvx/schema"
)

var messagesTestCmd = &cli.Command{
	Name:        "messages-test",
	Description: "",
	Flags:       []cli.Flag{&cidFlag, &apiFlag},
	Action:      runMessagesTest,
}

func runMessagesTest(c *cli.Context) error {
	v := newMessageVector()

	// input / output params
	existingActorType := address.SECP256K1
	existingActorBal := abi_spec.NewTokenAmount(10000000000)
	newActorAddr, _ := addr.NewSecp256k1Address([]byte("publickeyfoo"))
	newActorInitBal := abi_spec.NewTokenAmount(10000)
	expExitCode := exitcode_spec.Ok

	td := drivers.NewTestDriver()

	v.Pre.StateTree.CAR = td.MarshalState()
	v.Pre.StateTree.RootCID = td.MarshalStateRoot()

	existingAccountAddr, _ := td.NewAccountActor(existingActorType, existingActorBal)
	msg := td.MessageProducer.Transfer(existingAccountAddr, newActorAddr, chain.Value(newActorInitBal), chain.Nonce(0))
	result := td.ApplyFailure(
		msg,
		expExitCode,
	)

	var err error
	v.ApplyMessage, err = msg.Serialize()
	if err != nil {
		panic(err)
	}

	// new actor balance will only exist if message was applied successfully.
	if expExitCode.IsSuccess() {
		td.AssertBalance(newActorAddr, newActorInitBal)
		td.AssertBalance(existingAccountAddr, big_spec.Sub(big_spec.Sub(existingActorBal, result.Receipt.GasUsed.Big()), newActorInitBal))
	}

	v.Post.StateTree.CAR = td.MarshalState()
	v.Post.StateTree.RootCID = td.MarshalStateRoot()

	// encode and output
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&v); err != nil {
		return err
	}

	return nil
}

func newMessageVector() schema.TestVector {
	return schema.TestVector{
		Class:    schema.ClassMessage,
		Selector: "",
		Meta: &schema.Metadata{
			ID:      "TK",
			Version: "TK",
			Gen: schema.GenerationData{
				Source:  "TK",
				Version: "TK",
			},
		},
		Pre: &schema.Preconditions{
			StateTree: &schema.StateTree{
				//CAR:     preData,
				//RootCID: preRoot.String(),
			},
		},
		//ApplyMessage: msgBytes,
		Post: &schema.Postconditions{
			StateTree: &schema.StateTree{
				//CAR:     postData,
				//RootCID: postRoot.String(),
			},
		},
	}
}
