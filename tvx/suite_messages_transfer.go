package main

import (
	"os"

	address "github.com/filecoin-project/go-address"
	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	account_spec "github.com/filecoin-project/specs-actors/actors/builtin/account"

	require "github.com/stretchr/testify/require"

	builtin_spec "github.com/filecoin-project/specs-actors/actors/builtin"
	exitcode_spec "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	"github.com/filecoin-project/oni/tvx/chain"
	"github.com/filecoin-project/oni/tvx/drivers"
	"github.com/filecoin-project/oni/tvx/schema"
)

type valueTransferTestCases struct {
	desc string

	sender    address.Address
	senderBal big_spec.Int

	transferAmnt big_spec.Int

	receiver    address.Address
	receiverBal big_spec.Int

	code exitcode_spec.ExitCode
}

func MessageTest_ValueTransferSimple() error {
	alice := chain.MustNewSECP256K1Addr("1")
	bob := chain.MustNewSECP256K1Addr("2")
	const gasLimit = 1_000_000_000

	testCases := []valueTransferTestCases{
		{
			desc: "successfully transfer funds from sender to receiver",

			sender:    alice,
			senderBal: big_spec.NewInt(10 * gasLimit),

			transferAmnt: big_spec.NewInt(50),

			receiver:    bob,
			receiverBal: big_spec.Zero(),

			code: exitcode_spec.Ok,
		},
		{
			desc: "successfully transfer zero funds from sender to receiver",

			sender:    alice,
			senderBal: big_spec.NewInt(10 * gasLimit),

			transferAmnt: big_spec.NewInt(0),

			receiver:    bob,
			receiverBal: big_spec.Zero(),

			code: exitcode_spec.Ok,
		},
		{
			desc: "fail to transfer more funds than sender balance > 0",

			sender:    alice,
			senderBal: big_spec.NewInt(10 * gasLimit),

			transferAmnt: big_spec.NewInt(10*gasLimit - gasLimit + 1),

			receiver:    bob,
			receiverBal: big_spec.Zero(),

			code: exitcode_spec.SysErrInsufficientFunds,
		},
		{
			desc: "fail to transfer more funds than sender has when sender balance == zero",

			sender:    alice,
			senderBal: big_spec.NewInt(gasLimit),

			transferAmnt: big_spec.NewInt(1),

			receiver:    bob,
			receiverBal: big_spec.Zero(),

			code: exitcode_spec.SysErrInsufficientFunds,
		},
	}

	for _, tc := range testCases {
		err := func(testname string) error {
			td := drivers.NewTestDriver(schema.NewMessageTestVector())
			td.Vector.Meta.Desc = testname

			// Create the to and from actors with balance in the state tree
			_, _, err := td.State().CreateActor(builtin_spec.AccountActorCodeID, tc.sender, tc.senderBal, &account_spec.State{Address: tc.sender})
			require.NoError(drivers.T, err)
			if tc.sender.String() != tc.receiver.String() {
				_, _, err := td.State().CreateActor(builtin_spec.AccountActorCodeID, tc.receiver, tc.receiverBal, &account_spec.State{Address: tc.receiver})
				require.NoError(drivers.T, err)
			}

			sendAct, err := td.State().Actor(tc.sender)
			require.NoError(drivers.T, err)
			require.Equal(drivers.T, tc.senderBal.String(), sendAct.Balance().String())

			td.UpdatePreStateRoot()

			msg := td.MessageProducer.Transfer(tc.sender, tc.receiver, chain.Value(tc.transferAmnt), chain.Nonce(0))

			result := td.ApplyFailure(
				msg,
				tc.code,
			)

			// create a message to transfer funds from `to` to `from` for amount `transferAmnt` and apply it to the state tree
			// assert the actor balances changed as expected, the receiver balance should not change if transfer fails
			if tc.code.IsSuccess() {
				td.AssertBalance(tc.sender, big_spec.Sub(big_spec.Sub(tc.senderBal, tc.transferAmnt), result.Receipt.GasUsed.Big()))
				td.AssertBalance(tc.receiver, tc.transferAmnt)
			} else {
				if tc.code == exitcode_spec.SysErrInsufficientFunds {
					td.AssertBalance(tc.sender, big_spec.Sub(tc.senderBal, result.Receipt.GasUsed.Big()))
				} else {
					td.AssertBalance(tc.sender, tc.senderBal)
				}
			}

			td.MustSerialize(os.Stdout)

			return nil
		}(tc.desc)
		if err != nil {
			return err
		}
	}

	return nil
}

func MessageTest_ValueTransferAdvance() error {
	var aliceInitialBalance = abi_spec.NewTokenAmount(10_000_000_000)

	err := func(testname string) error {
		td := drivers.NewTestDriver(schema.NewMessageTestVector())
		td.Vector.Meta.Desc = testname

		alice, _ := td.NewAccountActor(drivers.SECP, aliceInitialBalance)
		transferAmnt := abi_spec.NewTokenAmount(10)

		td.UpdatePreStateRoot()

		msg := td.MessageProducer.Transfer(alice, alice, chain.Value(transferAmnt), chain.Nonce(0))
		result := td.ApplyOk(msg)

		// since this is a self transfer expect alice's balance to only decrease by the gasUsed
		td.AssertBalance(alice, big_spec.Sub(aliceInitialBalance, result.Receipt.GasUsed.Big()))

		td.MustSerialize(os.Stdout)

		return nil
	}("self transfer secp to secp")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver(schema.NewMessageTestVector())
		td.Vector.Meta.Desc = testname

		alice, aliceId := td.NewAccountActor(drivers.SECP, aliceInitialBalance)
		transferAmnt := abi_spec.NewTokenAmount(10)

		td.UpdatePreStateRoot()

		msg := td.MessageProducer.Transfer(alice, aliceId, chain.Value(transferAmnt), chain.Nonce(0))

		result := td.ApplyOk(msg)

		// since this is a self transfer expect alice's balance to only decrease by the gasUsed
		td.AssertBalance(alice, big_spec.Sub(aliceInitialBalance, result.Receipt.GasUsed.Big()))

		td.MustSerialize(os.Stdout)

		return nil
	}("self transfer secp to id address")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver(schema.NewMessageTestVector())
		td.Vector.Meta.Desc = testname

		alice, aliceId := td.NewAccountActor(drivers.SECP, aliceInitialBalance)
		transferAmnt := abi_spec.NewTokenAmount(10)

		td.UpdatePreStateRoot()

		msg := td.MessageProducer.Transfer(aliceId, alice, chain.Value(transferAmnt), chain.Nonce(0))

		result := td.ApplyOk(msg)

		// since this is a self transfer expect alice's balance to only decrease by the gasUsed
		td.AssertBalance(alice, big_spec.Sub(aliceInitialBalance, result.Receipt.GasUsed.Big()))

		td.MustSerialize(os.Stdout)

		return nil
	}("self transfer id to secp address")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver(schema.NewMessageTestVector())
		td.Vector.Meta.Desc = testname

		alice, aliceId := td.NewAccountActor(drivers.SECP, aliceInitialBalance)
		transferAmnt := abi_spec.NewTokenAmount(10)

		td.UpdatePreStateRoot()

		msg := td.MessageProducer.Transfer(aliceId, aliceId, chain.Value(transferAmnt), chain.Nonce(0))

		result := td.ApplyOk(msg)

		// since this is a self transfer expect alice's balance to only decrease by the gasUsed
		td.AssertBalance(alice, big_spec.Sub(aliceInitialBalance, result.Receipt.GasUsed.Big()))

		td.MustSerialize(os.Stdout)

		return nil
	}("self transfer id to id address")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver(schema.NewMessageTestVector())
		td.Vector.Meta.Desc = testname

		alice, _ := td.NewAccountActor(drivers.SECP, aliceInitialBalance)
		receiver := td.Wallet().NewSECP256k1AccountAddress()
		transferAmnt := abi_spec.NewTokenAmount(10)

		td.UpdatePreStateRoot()

		msg := td.MessageProducer.Transfer(alice, receiver, chain.Value(transferAmnt), chain.Nonce(0))

		result := td.ApplyOk(msg)
		td.AssertBalance(alice, big_spec.Sub(big_spec.Sub(aliceInitialBalance, result.Receipt.GasUsed.Big()), transferAmnt))
		td.AssertBalance(receiver, transferAmnt)

		td.MustSerialize(os.Stdout)

		return nil
	}("ok transfer from known address to new account")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver(schema.NewMessageTestVector())
		td.Vector.Meta.Desc = testname

		alice, _ := td.NewAccountActor(drivers.SECP, aliceInitialBalance)
		unknown := td.Wallet().NewSECP256k1AccountAddress()
		transferAmnt := abi_spec.NewTokenAmount(10)

		td.UpdatePreStateRoot()

		msg := td.MessageProducer.Transfer(unknown, alice, chain.Value(transferAmnt), chain.Nonce(0))

		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrSenderInvalid)
		td.AssertBalance(alice, aliceInitialBalance)

		td.MustSerialize(os.Stdout)

		return nil
	}("fail to transfer from unknown account to known address")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver(schema.NewMessageTestVector())
		td.Vector.Meta.Desc = testname

		sender := td.Wallet().NewSECP256k1AccountAddress()
		receiver := td.Wallet().NewSECP256k1AccountAddress()
		transferAmnt := abi_spec.NewTokenAmount(10)

		td.UpdatePreStateRoot()

		msg := td.MessageProducer.Transfer(sender, receiver, chain.Value(transferAmnt), chain.Nonce(0))

		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrSenderInvalid)
		td.AssertNoActor(sender)
		td.AssertNoActor(receiver)

		td.MustSerialize(os.Stdout)

		return nil
	}("fail to transfer from unknown address to unknown address")
	if err != nil {
		return err
	}

	return nil
}
