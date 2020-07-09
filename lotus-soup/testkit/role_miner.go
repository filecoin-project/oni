package testkit

import (
	"context"
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"net/http"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/filecoin-project/go-storedcounter"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apistruct"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/gen"
	genesis_chain "github.com/filecoin-project/lotus/chain/gen/genesis"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
	"github.com/filecoin-project/lotus/cmd/lotus-seed/seed"
	"github.com/filecoin-project/lotus/miner"
	"github.com/filecoin-project/lotus/node"
	"github.com/filecoin-project/lotus/node/impl"
	"github.com/filecoin-project/lotus/node/modules"
	"github.com/filecoin-project/lotus/node/modules/dtypes"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	saminer "github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/crypto"
	"github.com/gorilla/mux"
	"github.com/ipfs/go-datastore"
	libp2pcrypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/testground/sdk-go/sync"
	"go.uber.org/fx"

	"github.com/filecoin-project/oni/lotus-soup/statemachine"
)

type LotusMiner struct {
	*LotusNode

	t *TestEnvironment

	minerInstance *miner.Miner
}

func PrepareMiner(t *TestEnvironment) (*LotusMiner, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PrepareNodeTimeout)
	defer cancel()

	pubsubTracer, err := GetPubsubTracerMaddr(ctx, t)
	if err != nil {
		return nil, err
	}

	drandOpt, err := GetRandomBeaconOpts(ctx, t)
	if err != nil {
		return nil, err
	}

	// first create a wallet
	walletKey, err := wallet.GenerateKey(crypto.SigTypeBLS)
	if err != nil {
		return nil, err
	}

	// publish the account ID/balance
	balance := t.FloatParam("balance")
	balanceMsg := &InitialBalanceMsg{Addr: walletKey.Address, Balance: balance}
	t.SyncClient.Publish(ctx, BalanceTopic, balanceMsg)

	// create and publish the preseal commitment
	priv, _, err := libp2pcrypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	minerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		return nil, err
	}

	// pick unique sequence number for each miner, no matter in which group they are
	seq := t.SyncClient.MustSignalAndWait(ctx, StateMinerPickSeqNum, t.IntParam("miners"))

	minerAddr, err := address.NewIDAddress(genesis_chain.MinerStart + uint64(seq-1))
	if err != nil {
		return nil, err
	}

	presealDir, err := ioutil.TempDir("", "preseal")
	if err != nil {
		return nil, err
	}

	sectors := t.IntParam("sectors")
	genMiner, _, err := seed.PreSeal(minerAddr, abi.RegisteredSealProof_StackedDrg2KiBV1, 0, sectors, presealDir, []byte("TODO: randomize this"), &walletKey.KeyInfo, false)
	if err != nil {
		return nil, err
	}
	genMiner.PeerId = minerID

	t.RecordMessage("Miner Info: Owner: %s Worker: %s", genMiner.Owner, genMiner.Worker)

	presealMsg := &PresealMsg{Miner: *genMiner, Seqno: seq}
	t.SyncClient.Publish(ctx, PresealTopic, presealMsg)

	// then collect the genesis block and bootstrapper address
	genesisMsg, err := WaitForGenesis(t, ctx)
	if err != nil {
		return nil, err
	}

	// create the node
	// we need both a full node _and_ and storage miner node
	n := &LotusNode{}

	m := &LotusMiner{LotusNode: n, t: t}

	// prepare the repo for the storage miner
	n.MinerRepo = repo.NewMemory(nil)

	lr, err := n.MinerRepo.Lock(repo.StorageMiner)
	if err != nil {
		return nil, err
	}

	ks, err := lr.KeyStore()
	if err != nil {
		return nil, err
	}

	kbytes, err := priv.Bytes()
	if err != nil {
		return nil, err
	}

	err = ks.Put("libp2p-host", types.KeyInfo{
		Type:       "libp2p-host",
		PrivateKey: kbytes,
	})
	if err != nil {
		return nil, err
	}

	ds, err := lr.Datastore("/metadata")
	if err != nil {
		return nil, err
	}

	err = ds.Put(datastore.NewKey("miner-address"), minerAddr.Bytes())
	if err != nil {
		return nil, err
	}

	nic := storedcounter.New(ds, datastore.NewKey(modules.StorageCounterDSPrefix))
	for i := 0; i < (sectors + 1); i++ {
		_, err = nic.Next()
		if err != nil {
			return nil, err
		}
	}

	err = lr.Close()
	if err != nil {
		return nil, err
	}

	minerIP := t.NetClient.MustGetDataNetworkIP().String()

	if n.FullRepo == nil {
		n.FullRepo = repo.NewMemory(nil)
	}

	stop1, err := node.New(context.Background(),
		node.FullAPI(&n.FullApi),
		node.Online(),
		node.Repo(n.FullRepo),
		withGenesis(genesisMsg.Genesis),
		withApiEndpoint(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", t.PortNumber("node_rpc", "0"))),
		withListenAddress(minerIP),
		withBootstrapper(genesisMsg.Bootstrapper),
		withPubsubConfig(false, pubsubTracer),
		drandOpt,
	)
	if err != nil {
		return nil, err
	}

	// set the wallet
	err = n.setWallet(ctx, walletKey)
	if err != nil {
		stop1(context.TODO())
		return nil, err
	}

	minerOpts := []node.Option{
		node.StorageMiner(&n.MinerApi),
		node.Online(),
		node.Repo(n.MinerRepo),
		node.Override(new(api.FullNode), n.FullApi),
		withApiEndpoint(fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", t.PortNumber("miner_rpc", "0"))),
		withMinerListenAddress(minerIP),
	}

	if t.StringParam("mining_mode") == "natural" {
		// call through to default DI module, but capture a reference. gross, but seems to be the only way to get a direct ref to the miner.Miner
		providerHook := func (lc fx.Lifecycle, ds dtypes.MetadataDS, api api.FullNode, epp gen.WinningPoStProver) (*miner.Miner, error) {
			var err error
			m.minerInstance, err = modules.SetupBlockProducer(lc, ds, api, epp)
			return m.minerInstance, err
		}
		minerOpts = append(minerOpts, node.Override(new(*miner.Miner), providerHook))
	} else {
		mineBlock := make(chan func(bool, error))

		// wrap the DI module returned by miner.NewTestMiner and grab a ref to the returned miner.Miner
		providerHook := func(f api.FullNode, p gen.WinningPoStProver) *miner.Miner {
			m.minerInstance = miner.NewTestMiner(mineBlock, minerAddr)(f, p)
			return m.minerInstance
		}

		minerOpts = append(minerOpts,
			node.Override(new(*miner.Miner), providerHook))

		n.MineOne = func(ctx context.Context, cb func(bool, error)) error {
			select {
			case mineBlock <- cb:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	stop2, err := node.New(context.Background(), minerOpts...)
	if err != nil {
		stop1(context.TODO())
		return nil, err
	}
	n.StopFn = func(ctx context.Context) error {
		// TODO use a multierror for this
		t.RecordMessage("stopping storage miner")
		err2 := stop2(ctx)
		t.RecordMessage("storage miner stopped, stopping full node")
		err1 := stop1(ctx)
		t.RecordMessage("full node stopped")
		if err2 != nil {
			return err2
		}
		return err1
	}

	registerAndExportMetrics(minerAddr.String())

	// collect stats based on Travis' scripts
	if t.InitContext.GroupSeq == 1 {
		go collectStats(t, ctx, n.FullApi)
	}

	// Start listening on the full node.
	fullNodeNetAddrs, err := n.FullApi.NetAddrsListen(ctx)
	if err != nil {
		panic(err)
	}

	// err = n.MinerApi.NetConnect(ctx, fullNodeNetAddrs)
	// if err != nil {
	// 	panic(err)
	// }

	// add local storage for presealed sectors
	err = n.MinerApi.StorageAddLocal(ctx, presealDir)
	if err != nil {
		n.StopFn(context.TODO())
		return nil, err
	}

	// set the miner PeerID (first run only)
	minerIDEncoded, err := actors.SerializeParams(&saminer.ChangePeerIDParams{NewID: abi.PeerID(minerID)})
	if err != nil {
		return nil, err
	}

	changeMinerID := &types.Message{
		To:       minerAddr,
		From:     genMiner.Worker,
		Method:   builtin.MethodsMiner.ChangePeerID,
		Params:   minerIDEncoded,
		Value:    types.NewInt(0),
		GasPrice: types.NewInt(0),
		GasLimit: 1000000,
	}

	_, err = n.FullApi.MpoolPushMessage(ctx, changeMinerID)
	if err != nil {
		n.StopFn(context.TODO())
		return nil, err
	}

	t.RecordMessage("publish our address to the miners addr topic")
	minerActor, err := n.MinerApi.ActorAddress(ctx)
	if err != nil {
		return nil, err
	}

	minerNetAddrs, err := n.MinerApi.NetAddrsListen(ctx)
	if err != nil {
		return nil, err
	}

	t.SyncClient.MustPublish(ctx, MinersAddrsTopic, MinerAddressesMsg{
		FullNetAddrs:   fullNodeNetAddrs,
		MinerNetAddrs:  minerNetAddrs,
		MinerActorAddr: minerActor,
	})

	t.RecordMessage("connecting to all other miners")

	// densely connect the miner's full nodes.
	minerCh := make(chan *MinerAddressesMsg, 16)
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	t.SyncClient.MustSubscribe(sctx, MinersAddrsTopic, minerCh)
	for i := 0; i < t.IntParam("miners"); i++ {
		m := <-minerCh
		if m.MinerActorAddr == minerActor {
			// once I find myself, I stop connecting to others, to avoid a simopen problem.
			break
		}
		err := n.FullApi.NetConnect(ctx, m.FullNetAddrs)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to miner %s on: %v", m.MinerActorAddr, m.FullNetAddrs)
		}
		t.RecordMessage("connected to full node of miner %s on %v", m.MinerActorAddr, m.FullNetAddrs)

	}

	t.RecordMessage("waiting for all nodes to be ready")
	t.SyncClient.MustSignalAndWait(ctx, StateReady, t.TestInstanceCount)

	err = startFullNodeAPIServer(t, n.FullRepo, n.FullApi)
	if err != nil {
		return nil, err
	}

	err = startStorageMinerAPIServer(t, n.MinerRepo, n.MinerApi)
	if err != nil {
		return nil, err
	}

	return m, err
}

func (m *LotusMiner) RunDefault() error {
	var (
		t       = m.t
		clients = t.IntParam("clients")
		miners  = t.IntParam("miners")
	)

	t.RecordMessage("running miner")
	t.RecordMessage("block delay: %v", build.BlockDelaySecs)
	t.D().Gauge("miner.block-delay").Update(float64(build.BlockDelaySecs))

	ctx := context.Background()
	myActorAddr, err := m.MinerApi.ActorAddress(ctx)
	if err != nil {
		return err
	}

	// mine / stop mining
	mine := true
	done := make(chan struct{})

	if m.MineOne != nil {
		go func() {
			defer t.RecordMessage("shutting down mining")
			defer close(done)

			var i int
			for i = 0; mine; i++ {
				// synchronize all miners to mine the next block
				t.RecordMessage("synchronizing all miners to mine next block [%d]", i)
				stateMineNext := sync.State(fmt.Sprintf("mine-block-%d", i))
				t.SyncClient.MustSignalAndWait(ctx, stateMineNext, miners)

				ch := make(chan error)
				const maxRetries = 100
				success := false
				for retries := 0; retries < maxRetries; retries++ {
					err := m.MineOne(ctx, func(mined bool, err error) {
						if mined {
							t.D().Counter(fmt.Sprintf("block.mine,miner=%s", myActorAddr)).Inc(1)
						}
						ch <- err
					})
					if err != nil {
						panic(err)
					}

					miningErr := <-ch
					if miningErr == nil {
						success = true
						break
					}
					t.D().Counter("block.mine.err").Inc(1)
					t.RecordMessage("retrying block [%d] after %d attempts due to mining error: %s",
						i, retries, miningErr)
				}
				if !success {
					panic(fmt.Errorf("failed to mine block %d after %d retries", i, maxRetries))
				}
			}

			// signal the last block to make sure no miners are left stuck waiting for the next block signal
			// while the others have stopped
			stateMineLast := sync.State(fmt.Sprintf("mine-block-%d", i))
			t.SyncClient.MustSignalEntry(ctx, stateMineLast)
		}()
	} else {
		close(done)
	}

	if t.IsParamSet("suspend_events") {
		suspender := statemachine.NewSuspender(m, t.RecordMessage)
		go suspender.RunEvents(t.StringParam("suspend_events"))
	}

	// wait for a signal from all clients to stop mining
	err = <-t.SyncClient.MustBarrier(ctx, StateStopMining, clients).C
	if err != nil {
		return err
	}

	mine = false
	<-done

	t.SyncClient.MustSignalAndWait(ctx, StateDone, t.TestInstanceCount)
	return nil
}

func (m *LotusMiner) Halt() {
	m.t.RecordMessage("halting miner")

	if err := m.minerInstance.Stop(context.TODO()); err != nil {
		panic(err)
	}
	m.t.RecordMessage("miner halted")
}

func (m *LotusMiner) Resume() {
	m.t.RecordMessage("resuming miner")
	if err := m.minerInstance.Start(context.TODO()); err != nil {
		panic(fmt.Errorf("failed to resume miner: %s", err))
	}
	m.t.RecordMessage("miner resumed")
}

func startStorageMinerAPIServer(t *TestEnvironment, repo repo.Repo, minerApi api.StorageMiner) error {
	mux := mux.NewRouter()

	rpcServer := jsonrpc.NewServer()
	rpcServer.Register("Filecoin", apistruct.PermissionedStorMinerAPI(minerApi))

	mux.Handle("/rpc/v0", rpcServer)
	mux.PathPrefix("/remote").HandlerFunc(minerApi.(*impl.StorageMinerAPI).ServeRemote)
	mux.PathPrefix("/").Handler(http.DefaultServeMux) // pprof

	exporter, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "lotus",
	})
	if err != nil {
		return err
	}

	mux.Handle("/debug/metrics", exporter)

	ah := &auth.Handler{
		Verify: minerApi.AuthVerify,
		Next:   mux.ServeHTTP,
	}

	endpoint, err := repo.APIEndpoint()
	if err != nil {
		return fmt.Errorf("no API endpoint in repo: %w", err)
	}

	srv := &http.Server{Handler: ah}

	listenAddr, err := startServer(endpoint, srv)
	if err != nil {
		return fmt.Errorf("failed to start storage miner API endpoint: %w", err)
	}

	t.RecordMessage("started storage miner API server at %s", listenAddr)
	return nil
}
