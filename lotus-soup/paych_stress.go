package main

import (
	"context"
	"fmt"

	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
	"github.com/testground/sdk-go/sync"

	"github.com/filecoin-project/oni/lotus-soup/testkit"
)

var SendersDoneState = sync.State("senders-done")

func paychStress(t *testkit.TestEnvironment) error {
	// Dispatch/forward non-client roles to defaults.
	if t.Role != "client" {
		return testkit.HandleDefaultRole(t)
	}

	// This is a client role
	t.RecordMessage("running payments client")

	var (
		// lanes to open; vouchers will be distributed across these lanes in round-robin fashion
		laneCount = t.IntParam("lane_count")
		// increments in which to send payment vouchers
		increments = t.IntParam("increments")
	)

	ctx := context.Background()
	cl, err := testkit.PrepareClient(t)
	if err != nil {
		return err
	}

	// are we the receiver or a sender?
	mode := "sender"
	if t.TestGroupInstanceCount == 1 {
		mode = "receiver"
	}

	var clients []*testkit.ClientAddressesMsg
	sctx, cancel := context.WithCancel(ctx)
	clientsCh := make(chan *testkit.ClientAddressesMsg)
	t.SyncClient.MustSubscribe(sctx, testkit.ClientsAddrsTopic, clientsCh)
	for i := 0; i < t.TestGroupInstanceCount; i++ {
		clients = append(clients, <-clientsCh)
	}
	cancel()

	switch mode {
	case "receiver":
		// one receiver, everyone else is a sender.
		<-t.SyncClient.MustBarrier(ctx, SendersDoneState, t.TestGroupInstanceCount-1).C

	case "sender":
		t.RecordMessage("acting as sender")

		// we're going to lock up all our funds into this one payment channel.
		recv := clients[0]
		balance, err := cl.FullApi.WalletBalance(ctx, cl.Wallet.Address)
		if err != nil {
			return fmt.Errorf("failed to acquire wallet balance: %w", err)
		}

		t.RecordMessage("my balance: %d", balance)
		t.RecordMessage("creating payment channel; from=%s, to=%s, funds=%d", cl.Wallet.Address, recv.WalletAddr, balance)

		channel, err := cl.FullApi.PaychGet(ctx, cl.Wallet.Address, recv.WalletAddr, balance)
		if err != nil {
			return fmt.Errorf("failed to create payment channel: %w", err)
		}

		t.RecordMessage("payment channel created; addr=%s, msg_cid=%s", channel.Channel, channel.ChannelMessage)

		t.RecordMessage("allocating lanes; count=%d", laneCount)

		// allocate as many lanes as required
		var lanes []uint64
		for i := 0; i < laneCount; i++ {
			lane, err := cl.FullApi.PaychAllocateLane(ctx, channel.Channel)
			if err != nil {
				return fmt.Errorf("failed to allocate lane: %w", err)
			}
			lanes = append(lanes, lane)
		}

		t.RecordMessage("lanes allocated; count=%d", laneCount)
		t.RecordMessage("sending payments in round-robin fashion across lanes; increments=%d", increments)

		// start sending payments
		var (
			zero   = big.Zero()
			amount = big.NewInt(int64(increments))
		)

	Outer:
		for remaining := balance; remaining.GreaterThan(zero); {
			for _, lane := range lanes {
				voucher, err := cl.FullApi.PaychVoucherCreate(ctx, channel.Channel, amount, lane)
				if err != nil {
					return fmt.Errorf("failed to create voucher: %w", err)
				}
				_, err = cl.FullApi.PaychVoucherSubmit(ctx, channel.Channel, voucher)
				if err != nil {
					return fmt.Errorf("failed to submit voucher: %w", err)
				}
				remaining = types.BigSub(remaining, amount)
				if remaining.LessThanEqual(zero) {
					// we have no more funds remaining.
					break Outer
				}
			}
		}

		t.SyncClient.MustSignalEntry(ctx, SendersDoneState)
	}

	return nil

}
