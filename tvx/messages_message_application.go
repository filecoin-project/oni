package main

import (
	"encoding/json"
	"os"

	abi_spec "github.com/filecoin-project/specs-actors/actors/abi"
	big_spec "github.com/filecoin-project/specs-actors/actors/abi/big"
	paych_spec "github.com/filecoin-project/specs-actors/actors/builtin/paych"
	crypto_spec "github.com/filecoin-project/specs-actors/actors/crypto"
	exitcode_spec "github.com/filecoin-project/specs-actors/actors/runtime/exitcode"

	"github.com/filecoin-project/oni/tvx/chain"
	"github.com/filecoin-project/oni/tvx/drivers"
	"github.com/filecoin-project/oni/tvx/test-suites/utils"
)

func MessageTest_MessageApplicationEdgecases() error {
	var aliceBal = abi_spec.NewTokenAmount(1_000_000_000_000)
	var transferAmnt = abi_spec.NewTokenAmount(10)

	err := func(testname string) error {
		td := drivers.NewTestDriver()

		v := newEmptyMessageVector()
		v.Pre.StateTree.CAR = td.MarshalState()

		alice, _ := td.NewAccountActor(drivers.SECP, aliceBal)
		msg := td.MessageProducer.Transfer(alice, alice, chain.Value(transferAmnt), chain.Nonce(0), chain.GasPrice(1), chain.GasLimit(8))
		v.ApplyMessages = append(v.ApplyMessages, msg.MustSerialize())

		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrOutOfGas)

		v.Post.StateTree.CAR = td.MarshalState()

		// encode and output
		enc := json.NewEncoder(os.Stdout)
		if err := enc.Encode(&v); err != nil {
			return err
		}

		return nil
	}("fail to cover gas cost for message receipt on chain")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()

		v := newEmptyMessageVector()
		v.Pre.StateTree.CAR = td.MarshalState()

		alice, _ := td.NewAccountActor(drivers.SECP, aliceBal)
		// Expect Message application to fail due to lack of gas
		td.ApplyFailure(
			td.MessageProducer.Transfer(alice, alice, chain.Value(transferAmnt), chain.Nonce(0), chain.GasPrice(10), chain.GasLimit(1)),
			exitcode_spec.SysErrOutOfGas)

		// Expect Message application to fail due to lack of gas when sender is unknown
		unknown := utils.NewIDAddr(10000000)
		msg := td.MessageProducer.Transfer(unknown, alice, chain.Value(transferAmnt), chain.Nonce(0), chain.GasPrice(10), chain.GasLimit(1))
		v.ApplyMessages = append(v.ApplyMessages, msg.MustSerialize())

		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrOutOfGas)

		v.Post.StateTree.CAR = td.MarshalState()

		// encode and output
		enc := json.NewEncoder(os.Stdout)
		if err := enc.Encode(&v); err != nil {
			return err
		}

		return nil
	}("not enough gas to pay message on-chain-size cost")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()

		v := newEmptyMessageVector()
		v.Pre.StateTree.CAR = td.MarshalState()

		alice, _ := td.NewAccountActor(drivers.SECP, aliceBal)
		aliceNonce := uint64(0)
		aliceNonceF := func() uint64 {
			defer func() { aliceNonce++ }()
			return aliceNonce
		}
		newAccountA := utils.NewSECP256K1Addr("1")

		msg := td.MessageProducer.Transfer(alice, newAccountA, chain.Value(transferAmnt), chain.Nonce(aliceNonceF()))
		v.ApplyMessages = append(v.ApplyMessages, msg.MustSerialize())

		// get the "true" gas cost of applying the message
		result := td.ApplyOk(msg)

		// decrease the gas cost by `gasStep` for each apply and ensure `SysErrOutOfGas` is always returned.
		trueGas := int64(result.GasUsed())
		gasStep := int64(trueGas / 100)
		newAccountB := utils.NewSECP256K1Addr("2")
		for tryGas := trueGas - gasStep; tryGas > 0; tryGas -= gasStep {
			msg := td.MessageProducer.Transfer(alice, newAccountB, chain.Value(transferAmnt), chain.Nonce(aliceNonceF()), chain.GasPrice(1), chain.GasLimit(tryGas))
			v.ApplyMessages = append(v.ApplyMessages, msg.MustSerialize())

			td.ApplyFailure(
				msg,
				exitcode_spec.SysErrOutOfGas,
			)
		}

		v.Post.StateTree.CAR = td.MarshalState()

		// encode and output
		enc := json.NewEncoder(os.Stdout)
		if err := enc.Encode(&v); err != nil {
			return err
		}

		return nil
	}("fail not enough gas to cover account actor creation")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()

		v := newEmptyMessageVector()
		v.Pre.StateTree.CAR = td.MarshalState()

		alice, _ := td.NewAccountActor(drivers.SECP, aliceBal)

		msg := td.MessageProducer.Transfer(alice, alice, chain.Value(transferAmnt), chain.Nonce(1))
		v.ApplyMessages = append(v.ApplyMessages, msg.MustSerialize())

		// Expect Message application to fail due to callseqnum being invalid: 1 instead of 0
		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrSenderStateInvalid)

		unknown := utils.NewIDAddr(10000000)
		msg = td.MessageProducer.Transfer(unknown, alice, chain.Value(transferAmnt), chain.Nonce(1))
		v.ApplyMessages = append(v.ApplyMessages, msg.MustSerialize())

		// Expect message application to fail due to unknow actor when call seq num is also incorrect
		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrSenderInvalid)

		v.Post.StateTree.CAR = td.MarshalState()

		// encode and output
		enc := json.NewEncoder(os.Stdout)
		if err := enc.Encode(&v); err != nil {
			return err
		}

		return nil
	}("invalid actor CallSeqNum")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()

		v := newEmptyMessageVector()
		v.Pre.StateTree.CAR = td.MarshalState()

		const pcTimeLock = abi_spec.ChainEpoch(10)
		const pcLane = uint64(123)
		const pcNonce = uint64(1)
		var pcAmount = big_spec.NewInt(10)
		var initialBal = abi_spec.NewTokenAmount(200_000_000_000)
		var toSend = abi_spec.NewTokenAmount(10_000)
		var pcSig = &crypto_spec.Signature{
			Type: crypto_spec.SigTypeBLS,
			Data: []byte("Grrr im an invalid signature, I cause panics in the payment channel actor"),
		}

		// will create and send on payment channel
		sender, _ := td.NewAccountActor(drivers.SECP, initialBal)
		// will be receiver on paych
		receiver, receiverID := td.NewAccountActor(drivers.SECP, initialBal)

		// the _expected_ address of the payment channel
		paychAddr := utils.NewIDAddr(utils.IdFromAddress(receiverID) + 1)
		createRet := td.ComputeInitActorExecReturn(sender, 0, 0, paychAddr)

		msg := td.MessageProducer.CreatePaymentChannelActor(sender, receiver, chain.Value(toSend), chain.Nonce(0))
		v.ApplyMessages = append(v.ApplyMessages, msg.MustSerialize())

		td.ApplyExpect(
			msg,
			chain.MustSerialize(&createRet))

		msg = td.MessageProducer.PaychUpdateChannelState(sender, paychAddr, &paych_spec.UpdateChannelStateParams{
			Sv: paych_spec.SignedVoucher{
				ChannelAddr:     paychAddr,
				TimeLockMin:     pcTimeLock,
				TimeLockMax:     pcTimeLock,
				SecretPreimage:  nil,
				Extra:           nil,
				Lane:            pcLane,
				Nonce:           pcNonce,
				Amount:          pcAmount,
				MinSettleHeight: 0,
				Merges:          nil,
				Signature:       pcSig, // construct with invalid signature
			},
		}, chain.Nonce(1), chain.Value(big_spec.Zero()))
		v.ApplyMessages = append(v.ApplyMessages, msg.MustSerialize())

		// message application fails due to invalid argument (signature).
		td.ApplyFailure(
			msg,
			exitcode_spec.ErrIllegalArgument)

		v.Post.StateTree.CAR = td.MarshalState()

		// encode and output
		enc := json.NewEncoder(os.Stdout)
		if err := enc.Encode(&v); err != nil {
			return err
		}

		return nil
	}("abort during actor execution")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()

		v := newEmptyMessageVector()
		v.Pre.StateTree.CAR = td.MarshalState()

		alice, _ := td.NewAccountActor(drivers.SECP, aliceBal)

		msg := td.MessageProducer.MarketComputeDataCommitment(alice, alice, nil, chain.Nonce(0))
		v.ApplyMessages = append(v.ApplyMessages, msg.MustSerialize())

		// message application fails because ComputeDataCommitment isn't defined
		// on the recipient actor
		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrInvalidMethod)

		v.Post.StateTree.CAR = td.MarshalState()

		// encode and output
		enc := json.NewEncoder(os.Stdout)
		if err := enc.Encode(&v); err != nil {
			return err
		}

		return nil
	}("invalid method for receiver")
	if err != nil {
		return err
	}

	err = func(testname string) error {
		td := drivers.NewTestDriver()

		v := newEmptyMessageVector()
		v.Pre.StateTree.CAR = td.MarshalState()

		alice, _ := td.NewAccountActor(drivers.SECP, aliceBal)

		// Sending a message to non-existent ID address must produce an error.
		unknownA := utils.NewIDAddr(10000000)
		msg := td.MessageProducer.Transfer(alice, unknownA, chain.Value(transferAmnt), chain.Nonce(0))

		v.ApplyMessages = append(v.ApplyMessages, msg.MustSerialize())

		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrInvalidReceiver)

		// Sending a message to non-existing actor address must produce an error.
		unknownB := utils.NewActorAddr("1234")
		msg = td.MessageProducer.Transfer(alice, unknownB, chain.Value(transferAmnt), chain.Nonce(1))
		v.ApplyMessages = append(v.ApplyMessages, msg.MustSerialize())

		td.ApplyFailure(
			msg,
			exitcode_spec.SysErrInvalidReceiver)

		v.Post.StateTree.CAR = td.MarshalState()

		// encode and output
		enc := json.NewEncoder(os.Stdout)
		if err := enc.Encode(&v); err != nil {
			return err
		}

		return nil
	}("receiver ID/Actor address does not exist")
	if err != nil {
		return err
	}

	return nil

	// TODO more tests:
	// - missing/mismatched params for receiver
	// - various out-of-gas cases
}