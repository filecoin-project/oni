package main

import (
	"encoding/json"
	"os"

	"github.com/hashicorp/go-multierror"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"

	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	exitcode_spec "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	"github.com/filecoin-project/oni/tvx/chain"
	"github.com/filecoin-project/oni/tvx/drivers"
	"github.com/filecoin-project/oni/tvx/schema"
	utils "github.com/filecoin-project/oni/tvx/test-suites/utils"
)

var suiteMessagesCmd = &cli.Command{
	Name:        "suite-messages",
	Description: "",
	Action:      suiteMessages,
}

func suiteMessages(c *cli.Context) error {
	var err *multierror.Error
	err = multierror.Append(MessageTest_AccountActorCreation())
	err = multierror.Append(MessageTest_InitActorSequentialIDAddressCreate())
	err = multierror.Append(MessageTest_MessageApplicationEdgecases())
	return err.ErrorOrNil()
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
			td := drivers.NewTestDriver()

			v := newEmptyMessageVector()

			existingAccountAddr, _ := td.NewAccountActor(tc.existingActorType, tc.existingActorBal)

			v.Pre.StateTree.CAR = td.MarshalState()

			msg := td.MessageProducer.Transfer(existingAccountAddr, tc.newActorAddr, chain.Value(tc.newActorInitBal), chain.Nonce(0))
			b, err := msg.Serialize()
			if err != nil {
				return err
			}
			v.ApplyMessages = []schema.HexEncodedBytes{b}
			result := td.ApplyFailure(
				msg,
				tc.expExitCode,
			)

			// new actor balance will only exist if message was applied successfully.
			if tc.expExitCode.IsSuccess() {
				td.AssertBalance(tc.newActorAddr, tc.newActorInitBal)
				td.AssertBalance(existingAccountAddr, big_spec.Sub(big_spec.Sub(tc.existingActorBal, result.Receipt.GasUsed.Big()), tc.newActorInitBal))
			}

			v.Post.StateTree.CAR = td.MarshalState()

			// encode and output
			enc := json.NewEncoder(os.Stdout)
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

func MessageTest_InitActorSequentialIDAddressCreate() error {
	td := drivers.NewTestDriver()

	v := newEmptyMessageVector()

	var initialBal = abi_spec.NewTokenAmount(200_000_000_000)
	var toSend = abi_spec.NewTokenAmount(10_000)

	sender, _ := td.NewAccountActor(drivers.SECP, initialBal)

	receiver, receiverID := td.NewAccountActor(drivers.SECP, initialBal)

	firstPaychAddr := utils.NewIDAddr(utils.IdFromAddress(receiverID) + 1)
	secondPaychAddr := utils.NewIDAddr(utils.IdFromAddress(receiverID) + 2)

	firstInitRet := td.ComputeInitActorExecReturn(sender, 0, 0, firstPaychAddr)
	secondInitRet := td.ComputeInitActorExecReturn(sender, 1, 0, secondPaychAddr)

	v.Pre.StateTree.CAR = td.MarshalState()

	msg1 := td.MessageProducer.CreatePaymentChannelActor(sender, receiver, chain.Value(toSend), chain.Nonce(0))
	td.ApplyExpect(
		msg1,
		chain.MustSerialize(&firstInitRet),
	)

	b1, err := msg1.Serialize()
	if err != nil {
		return err
	}
	v.ApplyMessages = append(v.ApplyMessages, b1)

	msg2 := td.MessageProducer.CreatePaymentChannelActor(sender, receiver, chain.Value(toSend), chain.Nonce(1))
	td.ApplyExpect(
		msg2,
		chain.MustSerialize(&secondInitRet),
	)

	b2, err := msg2.Serialize()
	if err != nil {
		return err
	}
	v.ApplyMessages = append(v.ApplyMessages, b2)

	v.Post.StateTree.CAR = td.MarshalState()

	// encode and output
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(&v); err != nil {
		return err
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