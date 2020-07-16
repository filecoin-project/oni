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
// This function takes a channel (localEpochStartCh) where we expect to receive
// local signals to advance the clock to the next epoch. Such signals should
// correspond to some local event that implies readiness to advance, such as
// "mining round completed" events from synchronized mining.
//
// Upon receiving a local advance signal, it forwards the clock by build.BlockDelaySecs,
// thus progressing one chain epoch. It then signals this fact to all other
// nodes participating in the test via the synchronization service.
//
// Once all miners and clients have advanced, this function releases the new
// abi.ChainEpoch on the `globalEpochStartCh`, thus signalling that all nodes
// are ready to commence that chain epoch.
//
// Essentially this behaviour can be assimilated to a distributed sempaphore,
// where all miners and clients wait until they have locally advanced their
// clocks. Once they all signal that fact via the relevant sync topic, the
// semaphore fires and allows them all to proceed at once.
//
// All of this happens in a background goroutine.
func (n *LotusNode) SynchronizeClock(ctx context.Context, genesisTime time.Time, localEpochStartCh <-chan abi.ChainEpoch, globalEpochStartCh chan<- abi.ChainEpoch) {
	// replace the clock, setting it to the genesis timestamp epoch
	// we are now ready to mine (and receive) the first block.
	n.MockClock = clock.NewMock()
	build.Clock = n.MockClock

	// start at genesis + 100ms.
	n.MockClock.Set(genesisTime.Add(100 * time.Millisecond))

	var (
		id = fmt.Sprintf("%s_%d", n.t.TestGroupID, n.t.GroupSeq)

		minerCnt  = n.t.IntParam("miners")
		clientCnt = n.t.IntParam("clients")
		totalCnt  = minerCnt + clientCnt // miner is running a worker and a mining node.

		epoch         = abi.ChainEpoch(0) // current epoch
		epochInterval = time.Duration(build.BlockDelaySecs) * time.Second

		ready []string

		signalCh = make(chan *ClockSyncMsg, 128)
		sub      = n.t.SyncClient.MustSubscribe(ctx, ClockSyncTopic, signalCh)
	)

	go func() {
		defer close(globalEpochStartCh)

		for {
			select {
			case <-localEpochStartCh:
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
}
