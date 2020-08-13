package main

import (
	"os"

	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/filecoin-project/specs-actors/actors/builtin/init"
	"github.com/filecoin-project/specs-actors/actors/builtin/paych"
	"github.com/filecoin-project/specs-actors/actors/crypto"

	. "github.com/filecoin-project/oni/tvx/builders"
	"github.com/filecoin-project/oni/tvx/drivers"
	"github.com/filecoin-project/oni/tvx/schema"
)

// TODO -filter

var initialBal = abi.NewTokenAmount(200_000_000_000)
var toSend = abi.NewTokenAmount(10_000)

func main() {

}

func happyPathCreate() {
	metadata := &schema.Metadata{
		ID:      "paych-ctor-ok",
		Version: "1.00",
		Desc:    "happy path constructor",
	}

	var sender, receiver AddressHandle

	steps := &MessageVectorSteps{
		Preconditions: func(assert *Asserter, pb *Preconditions) {
			// Set up sender and receiver accounts.
			pb.AccountActors(drivers.SECP, initialBal, &sender, &receiver)
		},
		Messages: func(assert *Asserter, m *Messages) {
			// Construct the payment channel.
			m.CreatePaymentChannelActor(sender.Robust, receiver.Robust, Value(toSend))
		},
		Check: func(assert *Asserter, ma *StateChecker, rets []*vm.ApplyRet) {
			expectedActorAddr := AddressHandle{
				ID:     MustNewIDAddr(MustIDFromAddress(receiver.ID) + 1),
				Robust: sender.NextActorAddress(0, 0),
			}

			// Verify init actor return.
			var ret init.ExecReturn
			MustDeserialize(rets[0].Return, &ret)
			assert.Equal(expectedActorAddr.Robust, ret.RobustAddress)
			assert.Equal(expectedActorAddr.ID, ret.IDAddress)

			// Verify the paych state.
			var state paych.State
			actor := ma.LoadActorState(ret.RobustAddress, &state)
			assert.Equal(sender.ID, state.From)
			assert.Equal(receiver.ID, state.To)
			assert.Equal(toSend, actor.Balance)
		},
	}

	GenerateMessageVector(metadata, steps)

}

func happyPathUpdate() {
	metadata := &schema.Metadata{
		ID:      "paych-ctor-ok",
		Version: "1.00",
		Desc:    "happy path update",
	}

	const pcTimeLock = abi.ChainEpoch(0)
	const pcLane = uint64(123)
	const pcNonce = uint64(1)
	var pcAmount = big.NewInt(10)
	var pcSig = &crypto.Signature{
		Type: crypto.SigTypeBLS,
		Data: []byte("signature goes here"), // TODO may need to generate an actual signature
	}

	var sender, receiver AddressHandle
	var paychAddr AddressHandle

	steps := &MessageVectorSteps{
		Preconditions: func(assert *Asserter, pb *Preconditions) {
			// Set up sender and receiver accounts.
			pb.AccountActors(drivers.SECP, initialBal, &sender, &receiver)

			paychAddr = AddressHandle{
				ID:     MustNewIDAddr(MustIDFromAddress(receiver.ID) + 1),
				Robust: sender.NextActorAddress(0, 0),
			}
		},
		Messages: func(assert *Asserter, m *Messages) {
			// Construct the payment channel.
			m.CreatePaymentChannelActor(sender.Robust, receiver.Robust, Value(toSend))

			// Update the payment channel.
			m.Typed(sender.Robust, paychAddr.Robust, PaychUpdateChannelState(&paych.UpdateChannelStateParams{
				Sv: paych.SignedVoucher{
					ChannelAddr:     paychAddr.Robust,
					TimeLockMin:     pcTimeLock,
					TimeLockMax:     0, // TimeLockMax set to 0 means no timeout
					SecretPreimage:  nil,
					Extra:           nil,
					Lane:            pcLane,
					Nonce:           pcNonce,
					Amount:          pcAmount,
					MinSettleHeight: 0,
					Merges:          nil,
					Signature:       pcSig,
				}}), Nonce(1), Value(big.Zero()))

		},
		Check: func(assert *Asserter, ma *StateChecker, rets []*vm.ApplyRet) {
			// Verify init actor return.
			var ret init.ExecReturn
			MustDeserialize(rets[0].Return, &ret)
			assert.Equal(paychAddr.Robust, ret.RobustAddress)
			assert.Equal(paychAddr.ID, ret.IDAddress)

			// Verify the paych state.
			var state paych.State
			ma.LoadActorState(ret.RobustAddress, &state)
			assert.Len(state.LaneStates, 1)

			ls := state.LaneStates[0]
			assert.Equal(pcAmount, ls.Redeemed)
			assert.Equal(pcNonce, ls.Nonce)
			assert.Equal(pcLane, ls.ID)
		},
	}

	GenerateMessageVector(metadata, steps)

}

func happyPathCollect() {
	metadata := &schema.Metadata{
		ID:      "paych-ctor-ok",
		Version: "1.00",
		Desc:    "happy path collect",
	}

	const pcTimeLock = abi.ChainEpoch(0)
	const pcLane = uint64(123)
	const pcNonce = uint64(1)
	var pcAmount = big.NewInt(10)

	var sender, receiver AddressHandle
	var paychAddr AddressHandle

	steps := &MessageVectorSteps{
		Preconditions: func(assert *Asserter, pb *Preconditions) {
			// Set up sender and receiver accounts.
			pb.AccountActors(drivers.SECP, initialBal, &sender, &receiver)

			paychAddr = AddressHandle{
				ID:     MustNewIDAddr(MustIDFromAddress(receiver.ID) + 1),
				Robust: sender.NextActorAddress(0, 0),
			}

		},
		Messages: func(assert *Asserter, m *Messages) {
			// Construct the payment channel.
			m.CreatePaymentChannelActor(sender.Robust, receiver.Robust, Value(toSend))

			m.Typed(sender.Robust, paychAddr.Robust, PaychUpdateChannelState(&paych.UpdateChannelStateParams{
				Sv: paych.SignedVoucher{
					ChannelAddr:     paychAddr.Robust,
					TimeLockMin:     abi.ChainEpoch(0),
					TimeLockMax:     0, // TimeLockMax set to 0 means no timeout
					SecretPreimage:  nil,
					Extra:           nil,
					Lane:            1,
					Nonce:           1,
					Amount:          toSend, // the amount that can be redeemed by receiver,
					MinSettleHeight: 0,
					Merges:          nil,
					Signature: &crypto.Signature{
						Type: crypto.SigTypeBLS,
						Data: []byte("signature goes here"),
					},
				},
			}), Nonce(1), Value(big.Zero()))

			m.Typed(receiver.Robust, paychAddr.Robust, PaychSettle(nil), Value(big.Zero()), Nonce(0))

			// advance the epoch so the funds may be redeemed.
			m.Typed(receiver.Robust, paychAddr.Robust, PaychSettle(nil), Value(big.Zero()), Nonce(1), Epoch(paych.SettleDelay))
		},
		Check: func(assert *Asserter, ma *StateChecker, rets []*vm.ApplyRet) {



			// receiver_balance = initial_balance + paych_send - settle_paych_msg_gas - collect_paych_msg_gas
			td.AssertBalance(receiver, big.Sub(big.Sub(big.Add(toSend, initialBal), settleResult.Receipt.GasUsed.Big()), collectResult.Receipt.GasUsed.Big()))
			// the paych actor should have been deleted after the collect
			td.AssertNoActor(paychAddr)

			td.MustSerialize(os.Stdout)
		},
	}

	GenerateMessageVector(metadata, steps)


}
