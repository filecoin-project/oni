package main

import (
	"context"
	"time"

	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/oni/lotus-soup/testkit"
)

func spamAttack(t *testkit.TestEnvironment) error {
	switch t.Role {
	case "client":
		return dealsStress(t)
	case "spammer":
		return runSpammer(t)
	default:
		return testkit.HandleDefaultRole(t)
	}
}

func runSpammer(t *testkit.TestEnvironment) error {
	t.RecordMessage("running spammer")

	cl, err := testkit.PrepareClient(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	client := cl.FullApi

	zero := types.NewInt(0)
	one := types.NewInt(1)
	for {
		balance, err := client.WalletBalance(ctx, cl.Wallet.Address)
		if types.BigCmp(balance, zero) <= 0 {
			break
		}

		t.RecordMessage("sending funds to self")
		msg := &types.Message{
			From:     cl.Wallet.Address,
			To:       cl.Wallet.Address,
			Value:    one,
			GasLimit: 10000,
			GasPrice: one,
		}
		_, err = client.MpoolPushMessage(ctx, msg)
		if err != nil {
			return err
		}
		time.Sleep(100 * time.Microsecond)
	}

	t.SyncClient.MustSignalAndWait(ctx, testkit.StateDone, t.TestInstanceCount)
	return nil

}
