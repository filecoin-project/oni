package testkit

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/testground/sdk-go/sync"
)

// MinerSyncTopic is the topic where miner events are exchanged.
var MinerSyncTopic = sync.NewTopic("miner_sync", &MiningSyncMsg{})

// MiningSyncMsg represents a miner event.
type MiningSyncMsg struct {
	Addr     address.Address
	Produced bool // whether this miner produced a block.
}

// SynchronizeMiner synchronizes this miner with all other miners in this test,
// so that they're mining in perfect synchrony according to a globally-
// synchronized epoch clock.
//
// This function _receives_ "epoch start" signals and _emits_ "epoch end"
// signals:
//
//  * an epoch start signal attempts to locally mine a block, and publishes
//    a message on a sync topic indicating that we're done mining, and whether
//    we produced a block or not.
//  * all miners wait until their HEAD advances to the tipset that includes
//    all blocks that were produced in this epoch.
//  * once that happens, we emit an epoch end signal, which the caller can
//    interpret as a signal to advance the clock and begin a new epoch.
func (m *LotusMiner) SynchronizeMiner(ctx context.Context, epochStart <-chan abi.ChainEpoch, epochEnd chan<- abi.ChainEpoch) {
	var (
		minerCnt = m.t.IntParam("miners")
		produced = make(map[address.Address]struct{}, minerCnt) // addresses that produced a block in this epoch.

		epoch = abi.ChainEpoch(0)
		done  []address.Address

		signalCh = make(chan *MiningSyncMsg, 128)
		sub      = m.t.SyncClient.MustSubscribe(ctx, MinerSyncTopic, signalCh)
	)

	// obtain our address so we can broadcast it in signal messages.
	actorAddr, err := m.MinerApi.ActorAddress(ctx)
	if err != nil {
		panic("failed to obtain actor address: " + err.Error())
	}

	// function to mine a single round.
	mineRound := func(epoch abi.ChainEpoch) (produced bool) {
		// mining can fail, so we perform 100 retries.
		const retries = 100
		for attempt := 0; attempt < retries; attempt++ {
			var (
				mined bool
				err   error
				done  = make(chan struct{})
			)

			err = m.MineOne(ctx, func(m bool, e error) {
				mined, err = m, e
				close(done)
			})
			if err != nil {
				// this is a serious error, we didn't even attempt to mine a block; abort.
				panic(fmt.Errorf("failed to trigger mining a block: %s", err))
			}
			<-done

			if err == nil {
				m.t.RecordMessage("~~~ did we mine a block at epoch %d? %t ~~~", epoch, mined)
				if mined {
					m.t.D().Counter(fmt.Sprintf("block.mine,miner=%s", actorAddr)).Inc(1)
				}
				return mined
			}

			m.t.D().Counter("block.mine.err").Inc(1)
			m.t.RecordMessage("retrying block at height %d after %d attempts due to mining error: %s", epoch, attempt, err)
		}

		panic(fmt.Errorf("failed to mine a block after %d retries", retries))
	}

	go func() {
		defer m.t.RecordMessage("shutting down mining")

		for {
			select {
			case e := <-epochStart:
				m.t.RecordMessage("epoch started: %d; mining", e)
				epoch = e

				produced := mineRound(epoch)
				m.t.SyncClient.MustPublish(ctx, MinerSyncTopic, &MiningSyncMsg{
					Addr:     actorAddr,
					Produced: produced,
				})

			case s := <-signalCh:
				// this is a signal that a miner is done mining.
				done = append(done, s.Addr)
				if s.Produced {
					produced[s.Addr] = struct{}{}
				}

				if len(done) != minerCnt {
					// loop over until all miners have reported.
					continue
				}

				// we've now ascertained that all miners have reported.
				//
				// if this is a null round, wait for no HEAD changes.
				// otherwise, we wait for blocks to propagate and for our HEAD
				// to progress to a tipset that contains the blocks from all
				// miners that produced a block, at the chain height we expect.
				if len(produced) > 0 {
					ctx, cancel := context.WithCancel(context.Background())
					changes := m.ChainStore.SubHeadChanges(ctx)

					func() {
						defer cancel()

						for change := range changes {
						Change:
							for _, c := range change {
								if c.Val.Height() != epoch {
									continue
								}
								blocks := c.Val.Blocks()
								if len(blocks) != len(produced) {
									// this tipset still doesn't include all mined blocks.
									continue
								}
								for _, b := range blocks {
									if _, ok := produced[b.Miner]; !ok {
										continue Change // not the HEAD we're looking for.
									}
								}
								// the HEAD tipset includes all blocks mined in this round; we're done: release!
								return
							}
						}
					}()
				}

				// reset state in preparation for next round.
				done = done[0:0]
				for k := range produced {
					delete(produced, k)
				}

				// signal epoch end and loop over.
				epochEnd <- epoch

			case <-ctx.Done():
				return

			case <-sub.Done():
				panic("mining synchronization subscription died")
			}
		}
	}()
}
