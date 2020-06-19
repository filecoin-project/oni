package main

import (
	"context"
	"time"

	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

var PrepareEnvTimeOut = 2 * time.Minute

type Environment struct {
	runenv *runtime.RunEnv
	syncC  sync.Client
	netC   network.Client
	ipaddr string
	seqno  int
}

// prepareEnv prepares the network environment
func prepareEnv(runenv *runtime.RunEnv) *Environment {
	ctx, cancel := WithTimeout(context.Background(), PrepareEnvTimeOut)
	defer cancel()

	runenv.RecordMessage("binding sync service client")
	client := sync.MustBoundClient(ctx, runenv)

	netclient := network.NewClient(client, runenv)
	runenv.RecordMessage("initializing network")
	netclient.MustWaitNetworkInitialized(ctx)

	config := &network.Config{
		// Control the "default" network. At the moment, this is the only network.
		Network: "default",

		// Enable this network. Setting this to false will disconnect this test
		// instance from this network. You probably don't want to do that.
		Enable:        true,
		CallbackState: "network-configured",
	}

	runenv.RecordMessage("configuring network")
	netclient.MustConfigureNetwork(ctx, config)

	runenv.RecordMessage("allocating IP")
	seq := client.MustSignalAndWait(ctx, "ip-allocation", runenv.TestInstanceCount)

	// TODO figure out our IP address

	return &Environemt{
		runenv: runenv,
		syncC:  client,
		netC:   client,
		ipaddr: "TOOO",
		seqno:  seq,
	}
}
