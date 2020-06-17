package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/cors"

	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/wallet"
)

func main() {
	runtime.Invoke(run)
}

type Params struct {
	TestInstanceCount int
	TestRun           string
}

func run(runenv *runtime.RunEnv) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3000*time.Second)
	defer cancel()

	var genesisAddr string
	var genesisAddrTopic = sync.NewTopic("genesisAddr", &genesisAddr)

	var walletAddress string
	var walletAddressTopic = sync.NewTopic("walletAddresses", &walletAddress)

	withShaping := true

	if withShaping && !runenv.TestSidecar {
		return fmt.Errorf("Need sidecar to shape traffic")
	}

	client := sync.MustBoundClient(ctx, runenv)
	defer client.Close()

	runenv.RecordMessage("Waiting for network to be initialized")
	netclient := network.NewClient(client, runenv)
	netclient.MustWaitNetworkInitialized(ctx)
	runenv.RecordMessage("Network initialized")

	config := &network.Config{
		Network:       "default",
		Enable:        true,
		CallbackState: "network-configured",
	}

	//if withShaping {
	//bandwidth := runenv.SizeParam("bandwidth")
	//runenv.RecordMessage("Bandwidth: %v", bandwidth)
	//config.Default = network.LinkShape{
	//Bandwidth: bandwidth,
	//}
	//}

	runenv.RecordMessage("before netclient.MustConfigureNetwork")
	netclient.MustConfigureNetwork(ctx, config)
	runenv.RecordMessage("Network configured")

	// Get a sequence number
	runenv.RecordMessage("get a sequence number")
	seq := client.MustSignalAndWait(ctx, "ip-allocation", runenv.TestInstanceCount)
	runenv.RecordMessage("I am %d", seq)

	if seq >= 1<<16 {
		return fmt.Errorf("test-case only supports 2**16 instances")
	}

	ipC := byte((seq >> 8) + 1)
	ipD := byte(seq)

	subnet := runenv.TestSubnet.IPNet
	config.IPv4 = &subnet
	config.IPv4.IP = append(config.IPv4.IP[0:2:2], ipC, ipD)
	config.CallbackState = "ip-changed"

	runenv.RecordMessage("before reconfiguring network")
	netclient.MustConfigureNetwork(ctx, config)

	runenv.RecordMessage("Test subnet: %v", runenv.TestSubnet)
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		runenv.RecordMessage("IP: %v", addr)
	}

	// States
	nodeReadyState := sync.State("nodeReady")
	doneState := sync.State("done")

	mux := http.NewServeMux()

	go func() {
		handler := cors.Default().Handler(mux)
		log.Fatal(http.ListenAndServe(":9999", handler))
	}()

	// Serve token files
	mux.HandleFunc("/.lotus/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "no-cache")
		http.ServeFile(w, r, "/root/.lotus/token")
	})

	mux.HandleFunc("/.lotusstorage/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "no-cache")
		http.ServeFile(w, r, "/root/.lotusstorage/token")
	})

	fs := http.FileServer(http.Dir("/root/downloads"))
	mux.Handle("/downloads/", http.StripPrefix("/downloads/", fs))

	mux.HandleFunc("/params", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		params := Params{
			TestInstanceCount: runenv.TestInstanceCount,
			TestRun:           runenv.TestRun,
		}
		js, err := json.Marshal(params)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	})

	switch {
	case seq == 1: // genesis node
		runenv.RecordMessage("Genesis: %v", config.IPv4.IP)

		runenv.RecordMessage("Pre-seal some sectors")
		cmdPreseal := exec.Command(
			"/lotus/lotus-seed",
			"pre-seal",
			"--sector-size=2KiB",
			"--num-sectors=2",
		)
		// cmdPreseal.Env = append(os.Environ(), "GOLOG_LOG_LEVEL="+runenv.StringParam("log-level"))
		outfile, err := os.Create(runenv.TestOutputsPath + "/pre-seal.out")
		if err != nil {
			return err
		}
		defer outfile.Close()
		cmdPreseal.Stdout = outfile
		cmdPreseal.Stderr = outfile
		err = cmdPreseal.Run()
		if err != nil {
			return err
		}

		runenv.RecordMessage("Create localnet.json")
		cmdCreateLocalNetJSON := exec.Command(
			"/lotus/lotus-seed",
			"genesis",
			"new",
			"/root/localnet.json",
		)
		// cmdCreateLocalNetJSON.Env = append(os.Environ(), "GOLOG_LOG_LEVEL="+runenv.StringParam("log-level"))
		outfile, err = os.Create(runenv.TestOutputsPath + "/create-localnet-json.out")
		if err != nil {
			return err
		}
		defer outfile.Close()
		cmdCreateLocalNetJSON.Stdout = outfile
		cmdCreateLocalNetJSON.Stderr = outfile
		err = cmdCreateLocalNetJSON.Run()
		if err != nil {
			if runenv.BooleanParam("keep-alive") {
				runenv.RecordMessage("create localnet.json failed")
				select {}
			} else {
				return err
			}
		}

		runenv.RecordMessage("Add genesis miner")
		cmdAddGenesisMiner := exec.Command(
			"/lotus/lotus-seed",
			"genesis",
			"add-miner",
			"/root/localnet.json",
			"/root/.genesis-sectors/pre-seal-t01000.json",
		)
		// cmdAddGenesisMiner.Env = append(os.Environ(), "GOLOG_LOG_LEVEL="+runenv.StringParam("log-level"))
		outfile, err = os.Create(runenv.TestOutputsPath + "/add-genesis-miner.out")
		if err != nil {
			return err
		}
		defer outfile.Close()
		cmdAddGenesisMiner.Stdout = outfile
		cmdAddGenesisMiner.Stderr = outfile
		err = cmdAddGenesisMiner.Run()
		if err != nil {
			return err
		}

		runenv.RecordMessage("Start up the first node")
		cmdNodeInitial := exec.Command(
			"/lotus/lotus",
			"daemon",
			"--lotus-make-genesis=/root/dev.gen",
			"--genesis-template=/root/localnet.json",
			"--bootstrap=false",
		)
		// cmdNode.Env = append(os.Environ(), "GOLOG_LOG_LEVEL="+runenv.StringParam("log-level"))
		outfile, err = os.Create(runenv.TestOutputsPath + "/node-initial.out")
		if err != nil {
			return err
		}
		defer outfile.Close()
		cmdNodeInitial.Stdout = outfile
		cmdNodeInitial.Stderr = outfile
		err = cmdNodeInitial.Start()
		if err != nil {
			return err
		}

		err = setupIPFS(runenv, cmdNodeInitial)
		if err != nil {
			return err
		}

		runenv.RecordMessage("Import the genesis miner key")
		cmdImportGenesisMinerKey := exec.Command(
			"/lotus/lotus",
			"wallet",
			"import",
			"/root/.genesis-sectors/pre-seal-t01000.key",
		)
		// cmdImportGenesisMinerKey.Env = append(os.Environ(), "GOLOG_LOG_LEVEL="+runenv.StringParam("log-level"))
		outfile, err = os.Create(runenv.TestOutputsPath + "/import-genesis-miner-key.out")
		if err != nil {
			if runenv.BooleanParam("keep-alive") {
				runenv.RecordMessage("import genesis miner key failed")
				select {}
			} else {
				return err
			}
		}
		defer outfile.Close()
		cmdImportGenesisMinerKey.Stdout = outfile
		cmdImportGenesisMinerKey.Stderr = outfile
		err = cmdImportGenesisMinerKey.Run()
		if err != nil {
			if runenv.BooleanParam("keep-alive") {
				runenv.RecordMessage("import genesis miner key failed")
				select {}
			} else {
				return err
			}
		}

		// copy to /outputs
		runenv.RecordMessage("Copy .genesis-sectors to outputs")
		parts := strings.Split(`cp -r /root/.genesis-sectors `+runenv.TestOutputsPath+`/`, " ")
		head := parts[0]
		args := parts[1:len(parts)]
		exec.Command(head, args...).Run()
		parts = strings.Split(`cp -r /root/dev.gen `+runenv.TestOutputsPath+`/`, " ")
		head = parts[0]
		args = parts[1:len(parts)]
		exec.Command(head, args...).Run()
		parts = strings.Split(`cp -r /root/localnet.json `+runenv.TestOutputsPath+`/`, " ")
		head = parts[0]
		args = parts[1:len(parts)]
		exec.Command(head, args...).Run()

		//op, _ := jq.Parse(".t01000.Owner") // extract owner id
		//data, _ := ioutil.ReadFile(runenv.TestOutputsPath + "/.genesis-sectors/pre-seal-t01000.json")
		//value, _ := op.Apply(data)
		//owner := string(value)
		//owner = owner[1 : len(owner)-1]

		//runenv.RecordMessage("Owner: " + owner)
		runenv.RecordMessage("Set up the genesis miner")
		cmdSetupMiner := exec.Command(
			"/lotus/lotus-storage-miner",
			"init",
			"--genesis-miner",
			"--actor=t01000",
			"--sector-size=2KiB",
			"--pre-sealed-sectors=/root/.genesis-sectors",
			"--pre-sealed-metadata=/root/.genesis-sectors/pre-seal-t01000.json",
			"--nosync",
		)
		// cmdSetupMiner.Env = append(os.Environ(), "GOLOG_LOG_LEVEL="+runenv.StringParam("log-level"))
		outfile, err = os.Create(runenv.TestOutputsPath + "/miner-setup.out")
		if err != nil {
			return err
		}
		defer outfile.Close()
		cmdSetupMiner.Stdout = outfile
		cmdSetupMiner.Stderr = outfile
		err = cmdSetupMiner.Run()
		if err != nil {
			return err
		}

		runenv.RecordMessage("Start up the miner")
		cmdMiner := exec.Command(
			"/lotus/lotus-storage-miner",
			"run",
			"--nosync",
		)
		// cmdMiner.Env = append(os.Environ(), "GOLOG_LOG_LEVEL="+runenv.StringParam("log-level"))
		outfile, err = os.Create(runenv.TestOutputsPath + "/miner.out")
		if err != nil {
			return err
		}
		defer outfile.Close()
		cmdMiner.Stdout = outfile
		cmdMiner.Stderr = outfile
		err = cmdMiner.Start()
		if err != nil {
			if runenv.BooleanParam("keep-alive") {
				runenv.RecordMessage("genesis miner failed")
				select {}
			} else {
				return err
			}
		}

		delay := 15
		runenv.RecordMessage("Sleeping %v seconds", delay)
		time.Sleep(time.Duration(delay) * time.Second)

		// Serve /root/dev.gen file for other nodes to use as genesis
		mux.HandleFunc("/dev.gen", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, "/root/dev.gen")
		})

		api, closer, err := connectToAPI()
		if err != nil {
			return err
		}
		defer closer()

		var genesisAddrNet multiaddr.Multiaddr

		addrInfo, err := api.NetAddrsListen(ctx)
		if err != nil {
			return err
		}
		for _, addr := range addrInfo.Addrs {
			// runenv.Message("Listen addr: %v", addr.String())
			_, ip, err := manet.DialArgs(addr)
			if err != nil {
				return err
			}
			// runenv.Message("IP: %v", ip)
			// runenv.Message("Match: %v", config.IPv4.IP.String())
			if strings.Split(ip, ":")[0] == config.IPv4.IP.String() {
				genesisAddrNet = addr
			}
		}
		if genesisAddrNet == nil {
			return fmt.Errorf("Couldn't match genesis addr")
		}

		peerID, err := api.ID(ctx)
		if err != nil {
			return err
		}

		genesisAddr := fmt.Sprintf("%v/p2p/%v", genesisAddrNet.String(), peerID)
		runenv.RecordMessage("Genesis addr: %v", genesisAddr)

		runenv.RecordMessage("Publish to genesisAddrTopic")
		_, err = client.Publish(ctx, genesisAddrTopic, &genesisAddr)
		if err != nil {
			return err
		}

		_ = client.MustSignalAndWait(ctx, "genesis-ready", runenv.TestInstanceCount)
		runenv.RecordMessage("State: genesisReady")

		var localWalletAddr *address.Address
		addrs, err := api.WalletList(ctx)
		if err != nil {
			return err
		}

		for _, addr := range addrs {
			localWalletAddr = &addr
		}
		if localWalletAddr == nil {
			return fmt.Errorf("Couldn't find wallet")
		}

		runenv.RecordMessage("Setting default wallet: %v", *localWalletAddr)
		err = api.WalletSetDefault(ctx, *localWalletAddr)
		if err != nil {
			return err
		}

		subscribeCtx, cancelSub := context.WithCancel(ctx)
		walletAddressCh := make(chan *string)
		if _, err := client.Subscribe(subscribeCtx, walletAddressTopic, walletAddressCh); err != nil {
			return errors.Wrap(err, "publish/subscribe failure")
		}
		defer cancelSub()
		for i := 1; i < runenv.TestInstanceCount; i++ {
			select {
			case walletAddress := <-walletAddressCh:
				runenv.RecordMessage("Sending funds to wallet: %v", *walletAddress)

				// Send funds - see cli/send.go in Lotus
				toAddr, err := address.NewFromString(*walletAddress)
				if err != nil {
					return err
				}

				val, err := types.ParseFIL("1000000")
				if err != nil {
					return err
				}

				msg := &types.Message{
					From:     *localWalletAddr,
					To:       toAddr,
					Value:    types.BigInt(val),
					GasLimit: 1000,
					GasPrice: types.NewInt(0),
				}

				_, err = api.MpoolPushMessage(ctx, msg)
				if err != nil {
					return err
				}
			}
		}
		cancelSub()

		// Wait until nodeReady state is signalled by all secondary nodes
		runenv.RecordMessage("Waiting for nodeReady from other nodes")
		_, err = client.Barrier(ctx, nodeReadyState, runenv.TestInstanceCount-1)
		if err != nil {
			return err
		}
		runenv.RecordMessage("State: nodeReady from other nodes")

		// Signal we're done and everybody should shut down
		_, err = client.SignalEntry(ctx, doneState)
		if err != nil {
			return err
		}
		runenv.RecordMessage("State: done")

		runenv.RecordSuccess()

		if runenv.BooleanParam("keep-alive") {
			stallAndWatchTipsetHead(ctx, runenv, api, *localWalletAddr)
		}

	case seq >= 2: // additional nodes
		runenv.RecordMessage("Node: %v", seq)

		delayBetweenStarts := 10000
		delay := time.Duration((int(seq)-2)*delayBetweenStarts) * time.Millisecond
		runenv.RecordMessage("Delaying for %v ms", delay)
		time.Sleep(delay)

		// Wait until genesisReady state is signalled.
		runenv.RecordMessage("Waiting for genesisReady")
		_ = client.MustSignalAndWait(ctx, "genesis-ready", runenv.TestInstanceCount)
		runenv.RecordMessage("State: genesisReady")

		subnet := runenv.TestSubnet.IPNet
		genesisIPv4 := &subnet
		genesisIPv4.IP = append(genesisIPv4.IP[0:2:2], 1, 1)

		// Download dev.gen file from genesis node
		resp, err := http.Get(fmt.Sprintf("http://%v:9999/dev.gen", genesisIPv4.IP))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		outfile, err := os.Create("/root/dev.gen")
		if err != nil {
			return err
		}
		io.Copy(outfile, resp.Body)

		var genesisAddrInfo *peer.AddrInfo

		genesisAddrCh := make(chan *string)
		subscribeCtx, cancelSub := context.WithCancel(ctx)
		defer cancelSub()
		_, err = client.Subscribe(subscribeCtx, genesisAddrTopic, genesisAddrCh)
		if err != nil {
			return err
		}
		select {
		case genesisAddr := <-genesisAddrCh:
			genesisMultiaddr, err := multiaddr.NewMultiaddr(*genesisAddr)
			if err != nil {
				return err
			}
			genesisAddrInfo, err = peer.AddrInfoFromP2pAddr(genesisMultiaddr)
			if err != nil {
				return err
			}
		case <-time.After(40 * time.Second):
			return fmt.Errorf("timeout fetching genesisAddr")
		}

		runenv.RecordMessage("Start the node")
		cmdNodeInitial := exec.Command(
			"/lotus/lotus",
			"daemon",
			"--genesis=/root/dev.gen",
			"--bootstrap=false",
		)
		// cmdNode.Env = append(os.Environ(), "GOLOG_LOG_LEVEL="+runenv.StringParam("log-level"))
		outfile, err = os.Create(runenv.TestOutputsPath + "/node-initial.out")
		if err != nil {
			return err
		}
		defer outfile.Close()
		cmdNodeInitial.Stdout = outfile
		cmdNodeInitial.Stderr = outfile
		err = cmdNodeInitial.Start()
		if err != nil {
			return err
		}

		err = setupIPFS(runenv, cmdNodeInitial)
		if err != nil {
			return err
		}

		runenv.RecordMessage("Connect to API")
		api, closer, err := connectToAPI()
		if err != nil {
			return err
		}
		defer closer()

		peerID, err := api.ID(ctx)
		if err != nil {
			return err
		}
		runenv.RecordMessage("Peer ID: %v", peerID)

		runenv.RecordMessage("Connecting to Genesis Node: %v", *genesisAddrInfo)
		err = api.NetConnect(ctx, *genesisAddrInfo)
		if err != nil {
			return err
		}

		// FIXME: Watch sync state instead
		runenv.RecordMessage("Sleeping for 15 seconds to sync")
		time.Sleep(15 * time.Second)

		runenv.RecordMessage("Creating bls wallet")
		address, err := api.WalletNew(ctx, wallet.ActSigType("bls"))
		if err != nil {
			return err
		}
		walletAddress := address.String()
		runenv.RecordMessage("Wallet: %v", walletAddress)

		_, err = client.Publish(ctx, walletAddressTopic, &walletAddress)
		if err != nil {
			return err
		}

		localWalletAddr, err := api.WalletDefaultAddress(ctx)
		if err != nil {
			return err
		}

		count := 0
		for {
			balance, err := api.WalletBalance(ctx, address)
			if err != nil {
				return err
			}
			if balance.Sign() > 0 {
				break
			}
			time.Sleep(1 * time.Second)
			count++
			if count > 300 {
				if runenv.BooleanParam("keep-alive") {
					runenv.RecordMessage("Timeout waiting for funds transfer")
					select {}
				} else {
					return fmt.Errorf("Timeout waiting for funds transfer")
				}
			}
		}

		runenv.RecordMessage("Set up the miner")
		cmdSetupMiner := exec.Command(
			"/lotus/lotus-storage-miner",
			"init",
			"--owner="+walletAddress,
		)
		// cmdSetupMiner.Env = append(os.Environ(), "GOLOG_LOG_LEVEL="+runenv.StringParam("log-level"))
		outfile, err = os.Create(runenv.TestOutputsPath + "/miner-setup.out")
		if err != nil {
			return err
		}
		defer outfile.Close()
		cmdSetupMiner.Stdout = outfile
		cmdSetupMiner.Stderr = outfile
		err = cmdSetupMiner.Run()
		if err != nil {
			return err
		}

		runenv.RecordMessage("Start up the miner")
		cmdMiner := exec.Command(
			"/lotus/lotus-storage-miner",
			"run",
		)
		// cmdMiner.Env = append(os.Environ(), "GOLOG_LOG_LEVEL="+runenv.StringParam("log-level"))
		outfile, err = os.Create(runenv.TestOutputsPath + "/miner.out")
		if err != nil {
			return err
		}
		defer outfile.Close()
		cmdMiner.Stdout = outfile
		cmdMiner.Stderr = outfile
		err = cmdMiner.Start()
		if err != nil {
			return err
		}

		runenv.RecordSuccess()

		// Signal we're ready
		_, err = client.SignalEntry(ctx, nodeReadyState)
		if err != nil {
			return err
		}
		runenv.RecordMessage("State: nodeReady")

		// Wait until done state is signalled.
		runenv.RecordMessage("Waiting for done")
		_, err = client.Barrier(ctx, doneState, 1)
		if err != nil {
			return err
		}
		runenv.RecordMessage("State: done")

		if runenv.BooleanParam("keep-alive") {
			stallAndWatchTipsetHead(ctx, runenv, api, localWalletAddr)
		}

	default:
		return fmt.Errorf("Unexpected seq: %v", seq)
	}

	if runenv.BooleanParam("keep-alive") {
		select {}
	}

	return nil
}

func connectToAPI() (api.FullNode, jsonrpc.ClientCloser, error) {
	ma, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1234/http")
	if err != nil {
		return nil, nil, err
	}

	tokenContent, err := ioutil.ReadFile("/root/.lotus/token")
	if err != nil {
		return nil, nil, err
	}
	token := string(tokenContent)
	_, addr, err := manet.DialArgs(ma)
	if err != nil {
		return nil, nil, err
	}
	addr = "ws://" + addr + "/rpc/v0"

	headers := http.Header{}
	headers.Add("Authorization", "Bearer "+string(token))

	api, closer, err := client.NewFullNodeRPC(addr, headers)
	if err != nil {
		return nil, nil, err
	}

	return api, closer, nil
}

func stallAndWatchTipsetHead(ctx context.Context, runenv *runtime.RunEnv,
	api api.FullNode, address address.Address) error {
	for {
		tipset, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		balance, err := api.WalletBalance(ctx, address)
		if err != nil {
			return err
		}

		runenv.RecordMessage("Height: %v Balance: %v", tipset.Height(), balance)
		time.Sleep(30 * time.Second)
	}
}

func setupIPFS(runenv *runtime.RunEnv, cmdNodeInitial *exec.Cmd) error {
	fmt.Println("Setup IPFS")

	delay := 5
	runenv.RecordMessage("Sleeping %v seconds", delay)
	time.Sleep(time.Duration(delay) * time.Second)

	runenv.RecordMessage("Stopping node process (to reload config)")
	err := cmdNodeInitial.Process.Kill()
	if err != nil {
		return err
	}

	runenv.RecordMessage("Modify Lotus config")
	cmdModifyLotusConfig := exec.Command(
		"/usr/bin/perl",
		"-pi",
		"-e",
		"s/^#  UseIpfs = false$/  UseIpfs = true/; s/^#  IpfsUseForRetrieval = false$/  IpfsUseForRetrieval = true/",
		"/root/.lotus/config.toml",
	)
	outfile, err := os.Create(runenv.TestOutputsPath + "/modify-lotus-config.out")
	if err != nil {
		return err
	}
	defer outfile.Close()
	cmdModifyLotusConfig.Stdout = outfile
	cmdModifyLotusConfig.Stderr = outfile
	err = cmdModifyLotusConfig.Run()
	if err != nil {
		return err
	}

	runenv.RecordMessage("Initialize IPFS")
	cmdIpfsInit := exec.Command(
		"/go/bin/ipfs",
		"init",
	)
	outfile, err = os.Create(runenv.TestOutputsPath + "/ipfs-init.out")
	if err != nil {
		return err
	}
	defer outfile.Close()
	cmdIpfsInit.Stdout = outfile
	cmdIpfsInit.Stderr = outfile
	err = cmdIpfsInit.Run()
	if err != nil {
		return err
	}

	runenv.RecordMessage("IPFS: Configuring")

	ipfsConfigs := []string{
		"--json Experimental.GraphsyncEnabled true",
		"Addresses.API /ip4/0.0.0.0/tcp/5001",
		"Addresses.Gateway /ip4/0.0.0.0/tcp/8080",
		"--json API.HTTPHeaders.Access-Control-Allow-Origin [\"*\"]",
		"--json API.HTTPHeaders.Access-Control-Allow-Methods [\"PUT\",\"POST\",\"GET\"]",
	}

	outfile, err = os.Create(runenv.TestOutputsPath + "/ipfs-configs.out")
	if err != nil {
		return err
	}

	for _, configCommand := range ipfsConfigs {
		fmt.Fprintf(outfile, "Configuring: ipfs config %v\n", configCommand)
		args := strings.Split(configCommand, " ")
		cmdIpfsConfigCommand := exec.Command(
			"/go/bin/ipfs",
			append([]string{"config"}, args...)...,
		)
		cmdIpfsConfigCommand.Stdout = outfile
		cmdIpfsConfigCommand.Stderr = outfile
		err = cmdIpfsConfigCommand.Run()
		if err != nil {
			return err
		}
	}

	runenv.RecordMessage("Start IPFS")
	cmdIpfs := exec.Command(
		"/go/bin/ipfs",
		"daemon",
	)
	outfile, err = os.Create(runenv.TestOutputsPath + "/ipfs.out")
	if err != nil {
		return err
	}
	defer outfile.Close()
	cmdIpfs.Stdout = outfile
	cmdIpfs.Stderr = outfile
	err = cmdIpfs.Start()
	if err != nil {
		return err
	}

	delay = 5
	runenv.RecordMessage("Sleeping %v seconds", delay)
	time.Sleep(time.Duration(delay) * time.Second)

	runenv.RecordMessage("Restart node")
	cmdNode := exec.Command(
		"/lotus/lotus",
		"daemon",
	)
	outfile, err = os.Create(runenv.TestOutputsPath + "/node.out")
	if err != nil {
		return err
	}
	defer outfile.Close()
	cmdNode.Stdout = outfile
	cmdNode.Stderr = outfile
	err = cmdNode.Start()
	if err != nil {
		return err
	}

	delay = 5
	runenv.RecordMessage("Sleeping %v seconds", delay)
	time.Sleep(time.Duration(delay) * time.Second)

	return nil
}
