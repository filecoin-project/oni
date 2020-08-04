package main

import (
	"fmt"
	"encoding/json"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"

	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	exitcode_spec "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	"github.com/filecoin-project/oni/tvx/chain"
	"github.com/filecoin-project/oni/tvx/drivers"
	utils "github.com/filecoin-project/oni/tvx/test-suites/utils"
	"github.com/filecoin-project/oni/tvx/schema"
)

var suiteMessagesCmd = &cli.Command{
	Name:        "suite-messages",
	Description: "",
	Flags:       []cli.Flag{&cidFlag, &apiFlag},
	Action:      suiteMessages,
}

func suiteMessages(c *cli.Context) error {
  return MessageTest_AccountActorCreation()
}

func MessageTest_AccountActorCreation() error {
	testCases := []struct {
		desc string

		existingActorType address.Protocol
		existingActorBal  abi_spec.TokenAmount

		newActorAddr    address.Address
		newActorInitBal abi_spec.TokenAmount

		expExitCode exitcode_spec.ExitCode
	}{
		{
			"success create SECP256K1 account actor",
			address.SECP256K1,
			abi_spec.NewTokenAmount(10_000_000_000),

			utils.NewSECP256K1Addr("publickeyfoo"),
			abi_spec.NewTokenAmount(10_000),

			exitcode_spec.Ok,
		},
		{
			"success create BLS account actor",
			address.SECP256K1,
			abi_spec.NewTokenAmount(10_000_000_000),

			utils.NewBLSAddr(1),
			abi_spec.NewTokenAmount(10_000),

			exitcode_spec.Ok,
		},
		{
			"fail create SECP256K1 account actor insufficient balance",
			address.SECP256K1,
			abi_spec.NewTokenAmount(9_999),

			utils.NewSECP256K1Addr("publickeybar"),
			abi_spec.NewTokenAmount(10_000),

			exitcode_spec.SysErrSenderStateInvalid,
		},
		{
			"fail create BLS account actor insufficient balance",
			address.SECP256K1,
			abi_spec.NewTokenAmount(9_999),

			utils.NewBLSAddr(1),
			abi_spec.NewTokenAmount(10_000),

			exitcode_spec.SysErrSenderStateInvalid,
		},
		// TODO add edge case tests that have insufficient balance after gas fees
	}
	for _, tc := range testCases {
		err := func() error {
			fmt.Println()
			fmt.Println(tc.desc)
			fmt.Println("=====")

			td := drivers.NewTestDriver()

			v := newEmptyMessageVector()
			v.Pre.StateTree.CAR = td.MarshalState()
			v.Pre.StateTree.RootCID = td.MarshalStateRoot()

			existingAccountAddr, _ := td.NewAccountActor(tc.existingActorType, tc.existingActorBal)
			result := td.ApplyFailure(
				td.MessageProducer.Transfer(existingAccountAddr, tc.newActorAddr, chain.Value(tc.newActorInitBal), chain.Nonce(0)),
				tc.expExitCode,
			)

			// new actor balance will only exist if message was applied successfully.
			if tc.expExitCode.IsSuccess() {
				td.AssertBalance(tc.newActorAddr, tc.newActorInitBal)
				td.AssertBalance(existingAccountAddr, big_spec.Sub(big_spec.Sub(tc.existingActorBal, result.Receipt.GasUsed.Big()), tc.newActorInitBal))
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
		}()

		if err != nil {
			return err
		}
	}

	return nil
}

func newEmptyMessageVector() schema.TestVector {
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
			StateTree: &schema.StateTree{},
		},
		Post: &schema.Postconditions{
			StateTree: &schema.StateTree{},
		},
	}
}

