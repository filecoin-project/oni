package rfwp

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"time"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/oni/lotus-soup/testkit"
)

func RecoveryFromFailedWindowedPoStE2E(t *testkit.TestEnvironment) error {
	switch t.Role {
	case "bootstrapper":
		return testkit.HandleDefaultRole(t)
	case "client":
		return handleClient(t)
	case "miner":
		return handleMiner(t)
	case "miner-biserk":
		return handleMinerBiserk(t)
	}

	return fmt.Errorf("unknown role: %s", t.Role)
}

func handleMiner(t *testkit.TestEnvironment) error {
	m, err := testkit.PrepareMiner(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	myActorAddr, err := m.MinerApi.ActorAddress(ctx)
	if err != nil {
		return err
	}

	t.RecordMessage("running miner: %s", myActorAddr)

	// collect chain state
	go ChainState(t, m.LotusNode)

	time.Sleep(3600 * time.Second)

	t.SyncClient.MustSignalAndWait(ctx, testkit.StateDone, t.TestInstanceCount)
	return nil
}

func handleMinerBiserk(t *testkit.TestEnvironment) error {
	m, err := testkit.PrepareMiner(t)
	if err != nil {
		return err
	}

	ctx := context.Background()
	myActorAddr, err := m.MinerApi.ActorAddress(ctx)
	if err != nil {
		return err
	}

	t.RecordMessage("running biserk miner: %s", myActorAddr)
	// collect chain state
	go ChainState(t, m.LotusNode)

	time.Sleep(180 * time.Second)

	t.RecordMessage("shutting down biserk miner: %s", myActorAddr)

	ctxt, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err = m.StopFn(ctxt)
	if err != nil {
		return err
	}

	t.RecordMessage("shutdown biserk miner: %s", myActorAddr)

	time.Sleep(3600 * time.Second)

	t.SyncClient.MustSignalAndWait(ctx, testkit.StateDone, t.TestInstanceCount)
	return nil
}

func handleClient(t *testkit.TestEnvironment) error {
	cl, err := testkit.PrepareClient(t)
	if err != nil {
		return err
	}

	// This is a client role
	t.RecordMessage("running client")

	ctx := context.Background()
	client := cl.FullApi

	// collect chain state
	go ChainState(t, cl.LotusNode)

	// select a miner based on our GroupSeq (client 1 -> miner 1 ; client 2 -> miner 2)
	// this assumes that all miner instances receive the same sorted MinerAddrs slice
	minerAddr := cl.MinerAddrs[t.InitContext.GroupSeq-1]
	if err := client.NetConnect(ctx, minerAddr.MinerNetAddrs); err != nil {
		return err
	}
	t.D().Counter(fmt.Sprintf("send-data-to,miner=%s", minerAddr.MinerActorAddr)).Inc(1)

	t.RecordMessage("selected %s as the miner", minerAddr.MinerActorAddr)

	time.Sleep(2 * time.Second)

	// generate 1800 bytes of random data
	data := make([]byte, 1800)
	rand.New(rand.NewSource(time.Now().UnixNano())).Read(data)

	file, err := ioutil.TempFile("/tmp", "data")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())

	_, err = file.Write(data)
	if err != nil {
		return err
	}

	fcid, err := client.ClientImport(ctx, api.FileRef{Path: file.Name(), IsCAR: false})
	if err != nil {
		return err
	}
	t.RecordMessage("file cid: %s", fcid)

	// start deal
	t1 := time.Now()
	deal := testkit.StartDeal(ctx, minerAddr.MinerActorAddr, client, fcid.Root)
	t.RecordMessage("started deal: %s", deal)

	// TODO: this sleep is only necessary because deals don't immediately get logged in the dealstore, we should fix this
	time.Sleep(2 * time.Second)

	t.RecordMessage("waiting for deal to be sealed")
	testkit.WaitDealSealed(t, ctx, client, deal)
	t.D().ResettingHistogram("deal.sealed").Update(int64(time.Since(t1)))

	t.SyncClient.MustSignalEntry(ctx, testkit.StateStopMining)

	time.Sleep(180 * time.Second)

	carExport := true

	t.RecordMessage("trying to retrieve %s", fcid)
	info, err := client.ClientGetDealInfo(ctx, *deal)
	if err != nil {
		return err
	}

	testkit.RetrieveData(t, ctx, client, fcid.Root, &info.PieceCID, carExport, data)
	t.D().ResettingHistogram("deal.retrieved").Update(int64(time.Since(t1)))

	time.Sleep(10 * time.Second) // wait for metrics to be emitted

	// TODO broadcast published content CIDs to other clients
	// TODO select a random piece of content published by some other client and retrieve it

	t.SyncClient.MustSignalAndWait(ctx, testkit.StateDone, t.TestInstanceCount)
	return nil
}
