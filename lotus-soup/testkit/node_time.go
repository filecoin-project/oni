package testkit

import (
	"context"
	"fmt"
	"time"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/raulk/clock"
	"github.com/testground/sdk-go/sync"
)

// ClockSyncTopic is the topic where clock synchronization events are exchanged.
var ClockSyncTopic = sync.NewTopic("clock_sync", &ClockSyncMsg{})

// ClockSyncMsg is a clock synchronization message.
type ClockSyncMsg struct {
	ID    string // ${groupname}_${groupseq}
	Epoch abi.ChainEpoch
}

// SynchronizeClock synchronizes this instance's clock with the clocks of all
// miner and client nodes participating in this test scenario.
//
// This function returns two channels.
//
// The first channel (localAdvance) is where the user requests local advancement
// to the next epoch. Such requests likely originate from some local event that
// implies readiness to advance, such as "mining round completed" events from
// synchronized mining. Upon receiving a local advance signal, the control
// goroutine forwards the local clock by build.BlockDelaySecs, thus progressing
// one chain epoch. It then signals this fact to all other nodes participating
// in the test via the Testground synchronization service.
//
// The second channel is where the global clock synchronizer signals every time
// the entirety of test participants have progressed to the next epoch, thus
// indicating that all nodes are eady to commence that chain epoch of
// computation.
//
// Essentially this behaviour can be assimilated to a distributed sempaphore.
//
// This method will return immediately, and will spawn a background goroutine
// that performs the clock synchronisation work.
func (n *LotusNode) SynchronizeClock(ctx context.Context) (localAdvance chan<- abi.ChainEpoch, globalEpoch <-chan abi.ChainEpoch, err error) {
	n.MockClock = clock.NewMock()
	build.Clock = n.MockClock

	gen, err := n.FullApi.ChainGetGenesis(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get genesis: %w", err)
	}

	// start at genesis.
	genesisTime := time.Unix(int64(gen.MinTimestamp()), 0)
	n.MockClock.Set(genesisTime)

	var (
		localEpochAdvanceCh = make(chan abi.ChainEpoch, 128)
		globalEpochStartCh  = make(chan abi.ChainEpoch, 128)
	)

	// jumpstart the clock!
	localEpochAdvanceCh <- abi.ChainEpoch(1)

	var (
		id = fmt.Sprintf("%s_%d", n.t.TestGroupID, n.t.GroupSeq)

		minerCnt  = n.t.IntParam("miners")
		clientCnt = n.t.IntParam("clients")
		totalCnt  = minerCnt + clientCnt

		epoch         = abi.ChainEpoch(0) // current epoch: genesis
		epochInterval = time.Duration(build.BlockDelaySecs) * time.Second

		// ready tracks which nodes have advanced to the next round.
		// when all nodes have advanced, we tick the global clock and reset
		// this slice.
		ready []string

		signalCh = make(chan *ClockSyncMsg, 128)
		sub      = n.t.SyncClient.MustSubscribe(ctx, ClockSyncTopic, signalCh)
	)

	go func() {
		defer close(globalEpochStartCh)

		for {
			select {
			case <-localEpochAdvanceCh:
				// move the local clock forward, and announce that fact to all other instances.
				n.MockClock.Add(epochInterval)
				epoch++
				n.t.RecordMessage("advanced local clock: %v [epoch=%d]", n.MockClock.Now(), epoch)
				n.t.SyncClient.MustPublish(ctx, ClockSyncTopic, &ClockSyncMsg{
					ID:    id,
					Epoch: epoch,
				})

			case s := <-signalCh:
				// this is a clock event from another instance (or ourselves).
				ready = append(ready, s.ID)
				if len(ready) != totalCnt {
					continue
				}
				// release the new epoch and reset the state.
				globalEpochStartCh <- epoch
				ready = ready[0:0]

			case <-ctx.Done():
				return

			case <-sub.Done():
				panic("global clock synchronization subscription died")
			}
		}
	}()

	return localEpochAdvanceCh, globalEpochStartCh, nil
}
