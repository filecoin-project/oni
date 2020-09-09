package testkit

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/beacon"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/metrics"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	modtest "github.com/filecoin-project/lotus/node/modules/testing"
	tstats "github.com/filecoin-project/lotus/tools/stats"
	"github.com/ipfs/go-cid"

	cbg "github.com/whyrusleeping/cbor-gen"

	influxdb "github.com/kpacha/opencensus-influxdb"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/actors/util/adt"
)

var PrepareNodeTimeout = time.Minute

type LotusNode struct {
	FullApi  api.FullNode
	MinerApi api.StorageMiner
	StopFn   node.StopFunc
	Wallet   *wallet.Key
	MineOne  func(context.Context, miner.MineReq) error
}

func (n *LotusNode) setWallet(ctx context.Context, walletKey *wallet.Key) error {
	_, err := n.FullApi.WalletImport(ctx, &walletKey.KeyInfo)
	if err != nil {
		return err
	}

	err = n.FullApi.WalletSetDefault(ctx, walletKey.Address)
	if err != nil {
		return err
	}

	n.Wallet = walletKey

	return nil
}

func WaitForBalances(t *TestEnvironment, ctx context.Context, nodes int) ([]*InitialBalanceMsg, error) {
	ch := make(chan *InitialBalanceMsg)
	sub := t.SyncClient.MustSubscribe(ctx, BalanceTopic, ch)

	balances := make([]*InitialBalanceMsg, 0, nodes)
	for i := 0; i < nodes; i++ {
		select {
		case m := <-ch:
			balances = append(balances, m)
		case err := <-sub.Done():
			return nil, fmt.Errorf("got error while waiting for balances: %w", err)
		}
	}

	return balances, nil
}

func CollectPreseals(t *TestEnvironment, ctx context.Context, miners int) ([]*PresealMsg, error) {
	ch := make(chan *PresealMsg)
	sub := t.SyncClient.MustSubscribe(ctx, PresealTopic, ch)

	preseals := make([]*PresealMsg, 0, miners)
	for i := 0; i < miners; i++ {
		select {
		case m := <-ch:
			preseals = append(preseals, m)
		case err := <-sub.Done():
			return nil, fmt.Errorf("got error while waiting for preseals: %w", err)
		}
	}

	sort.Slice(preseals, func(i, j int) bool {
		return preseals[i].Seqno < preseals[j].Seqno
	})

	return preseals, nil
}

func WaitForGenesis(t *TestEnvironment, ctx context.Context) (*GenesisMsg, error) {
	genesisCh := make(chan *GenesisMsg)
	sub := t.SyncClient.MustSubscribe(ctx, GenesisTopic, genesisCh)

	select {
	case genesisMsg := <-genesisCh:
		return genesisMsg, nil
	case err := <-sub.Done():
		return nil, fmt.Errorf("error while waiting for genesis msg: %w", err)
	}
}

func CollectMinerAddrs(t *TestEnvironment, ctx context.Context, miners int) ([]MinerAddressesMsg, error) {
	ch := make(chan MinerAddressesMsg)
	sub := t.SyncClient.MustSubscribe(ctx, MinersAddrsTopic, ch)

	addrs := make([]MinerAddressesMsg, 0, miners)
	for i := 0; i < miners; i++ {
		select {
		case a := <-ch:
			addrs = append(addrs, a)
		case err := <-sub.Done():
			return nil, fmt.Errorf("got error while waiting for miners addrs: %w", err)
		}
	}

	return addrs, nil
}

func CollectClientAddrs(t *TestEnvironment, ctx context.Context, clients int) ([]*ClientAddressesMsg, error) {
	ch := make(chan *ClientAddressesMsg)
	sub := t.SyncClient.MustSubscribe(ctx, ClientsAddrsTopic, ch)

	addrs := make([]*ClientAddressesMsg, 0, clients)
	for i := 0; i < clients; i++ {
		select {
		case a := <-ch:
			addrs = append(addrs, a)
		case err := <-sub.Done():
			return nil, fmt.Errorf("got error while waiting for clients addrs: %w", err)
		}
	}

	return addrs, nil
}

func GetPubsubTracerMaddr(ctx context.Context, t *TestEnvironment) (string, error) {
	if !t.BooleanParam("enable_pubsub_tracer") {
		return "", nil
	}

	ch := make(chan *PubsubTracerMsg)
	sub := t.SyncClient.MustSubscribe(ctx, PubsubTracerTopic, ch)

	select {
	case m := <-ch:
		return m.Multiaddr, nil
	case err := <-sub.Done():
		return "", fmt.Errorf("got error while waiting for pubsub tracer config: %w", err)
	}
}

func GetRandomBeaconOpts(ctx context.Context, t *TestEnvironment) (node.Option, error) {
	beaconType := t.StringParam("random_beacon_type")
	switch beaconType {
	case "external-drand":
		noop := func(settings *node.Settings) error {
			return nil
		}
		return noop, nil

	case "local-drand":
		cfg, err := waitForDrandConfig(ctx, t.SyncClient)
		if err != nil {
			t.RecordMessage("error getting drand config: %w", err)
			return nil, err

		}
		t.RecordMessage("setting drand config: %v", cfg)
		return node.Options(
			node.Override(new(dtypes.DrandConfig), cfg.Config),
			node.Override(new(dtypes.DrandBootstrap), cfg.GossipBootstrap),
		), nil

	case "mock":
		return node.Options(
			node.Override(new(beacon.RandomBeacon), modtest.RandomBeacon),
			node.Override(new(dtypes.DrandConfig), dtypes.DrandConfig{
				ChainInfoJSON: "{\"Hash\":\"wtf\"}",
			}),
			node.Override(new(dtypes.DrandBootstrap), dtypes.DrandBootstrap{}),
		), nil

	default:
		return nil, fmt.Errorf("unknown random_beacon_type: %s", beaconType)
	}
}

func startServer(endpoint ma.Multiaddr, srv *http.Server) (listenAddr string, err error) {
	lst, err := manet.Listen(endpoint)
	if err != nil {
		return "", fmt.Errorf("could not listen: %w", err)
	}

	go func() {
		_ = srv.Serve(manet.NetListener(lst))
	}()

	return lst.Addr().String(), nil
}

func registerAndExportMetrics(instanceName string) {
	// Register all Lotus metric views
	err := view.Register(metrics.DefaultViews...)
	if err != nil {
		panic(err)
	}

	// Set the metric to one so it is published to the exporter
	stats.Record(context.Background(), metrics.LotusInfo.M(1))

	// Register our custom exporter to opencensus
	e, err := influxdb.NewExporter(context.Background(), influxdb.Options{
		Database:     "testground",
		Address:      os.Getenv("INFLUXDB_URL"),
		Username:     "",
		Password:     "",
		InstanceName: instanceName,
	})
	if err != nil {
		panic(err)
	}
	view.RegisterExporter(e)
	view.SetReportingPeriod(5 * time.Second)
}

func collectStats(t *TestEnvironment, ctx context.Context, api api.FullNode) error {
	t.RecordMessage("collecting blockchain stats")

	influxAddr := os.Getenv("INFLUXDB_URL")
	influxUser := ""
	influxPass := ""
	influxDb := "testground"

	influx, err := tstats.InfluxClient(influxAddr, influxUser, influxPass)
	if err != nil {
		t.RecordMessage(err.Error())
		return err
	}

	height := int64(0)
	headlag := 1

	go func() {
		time.Sleep(15 * time.Second)
		t.RecordMessage("calling tstats.Collect")
		tstats.Collect(context.Background(), api, influx, influxDb, height, headlag)
	}()

	return nil
}

func monitorChainAndPower(t *TestEnvironment, ctx context.Context, api api.FullNode) error {
	go func() {
		time.Sleep(20 * time.Second)
		t.RecordMessage("monitor CP")
		err := monitorCP(t, ctx, api)
		if err != nil {
			t.RecordMessage(err.Error())
		}
	}()

	return nil
}

func monitorCP(t *TestEnvironment, ctx context.Context, api api.FullNode) error {
	h := 0
	headlag := 1

	tipsetsCh, err := tstats.GetTips(ctx, api, abi.ChainEpoch(h), headlag)
	if err != nil {
		return err
	}

	for tipset := range tipsetsCh {
		t.RecordMessage("Collect stats", "height", tipset.Height())

		attoFil := types.NewInt(build.FilecoinPrecision).Int

		netBal, err := api.WalletBalance(ctx, builtin.RewardActorAddr)
		if err != nil {
			return err
		}

		netBalFil := new(big.Rat).SetFrac(netBal.Int, attoFil)
		netBalFilFloat, _ := netBalFil.Float64()
		t.RecordMessage("network.balance", netBalFilFloat)

		totalPower, err := api.StateMinerPower(ctx, address.Address{}, tipset.Key())
		if err != nil {
			return err
		}
		t.RecordMessage("chain.power", totalPower.TotalPower.QualityAdjPower.Int64())

		powerActor, err := api.StateGetActor(ctx, builtin.StoragePowerActorAddr, tipset.Key())
		if err != nil {
			return err
		}

		powerRaw, err := api.ChainReadObj(ctx, powerActor.Head)
		if err != nil {
			return err
		}

		var powerActorState power.State

		if err := powerActorState.UnmarshalCBOR(bytes.NewReader(powerRaw)); err != nil {
			return fmt.Errorf("failed to unmarshal power actor state: %w", err)
		}

		s := &apiIpldStore{ctx, api}
		mp, err := adt.AsMap(s, powerActorState.Claims)
		if err != nil {
			return err
		}

		var claim power.Claim
		err = mp.ForEach(&claim, func(key string) error {
			addr, err := address.NewFromBytes([]byte(key))
			if err != nil {
				return err
			}

			if claim.QualityAdjPower.Int64() == 0 {
				return nil
			}

			t.RecordMessage("chain.miner_power", claim.QualityAdjPower.Int64(), "miner", addr.String())

			return nil
		})
		if err != nil {
			return err
		}

		return nil
	}

	return nil
}

type apiIpldStore struct {
	ctx context.Context
	api api.FullNode
}

func (ht *apiIpldStore) Context() context.Context {
	return ht.ctx
}

func (ht *apiIpldStore) Get(ctx context.Context, c cid.Cid, out interface{}) error {
	raw, err := ht.api.ChainReadObj(ctx, c)
	if err != nil {
		return err
	}

	cu, ok := out.(cbg.CBORUnmarshaler)
	if ok {
		if err := cu.UnmarshalCBOR(bytes.NewReader(raw)); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("Object does not implement CBORUnmarshaler")
}

func (ht *apiIpldStore) Put(ctx context.Context, v interface{}) (cid.Cid, error) {
	return cid.Undef, fmt.Errorf("Put is not implemented on apiIpldStore")
}
