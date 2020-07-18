package testkit

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/sync"
)

// MinerSyncTopic is the topic where miner events are exchanged.
var MinerSyncTopic = sync.NewTopic("miner_sync", &MiningSyncMsg{})

// MiningSyncMsg signals that the miner attempted to mine in a particular epoch,
// indicating whether it produced a block or not.
type MiningSyncMsg struct {
	// Addr is the address of the miner.
	Addr address.Address
	// Produced signals whether the miner produced a block or not.
	Produced bool
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
func (m *LotusMiner) SynchronizeMiner(ctx context.Context, mineTrigger <-chan abi.ChainEpoch, mineDone chan<- abi.ChainEpoch) {
	defer run.HandlePanics()
	defer m.t.RecordMessage("shutting down mining")

	var (
		epoch = abi.ChainEpoch(0)

		minerCnt = m.t.IntParam("miners")
		// done tracks the miners that are done producing a block in this epoch.
		done []address.Address
		// produced indexes the miners that produced a block in this epoch.
		produced = make(map[address.Address]struct{}, minerCnt)

		signalCh = make(chan *MiningSyncMsg, 128)
		sub      = m.t.SyncClient.MustSubscribe(ctx, MinerSyncTopic, signalCh)
	)

	for {
		select {
		case e := <-mineTrigger:
			m.t.RecordMessage("epoch started: %d; mining", e)
			epoch = e

			produced, err := m.mineRound(ctx, epoch)
			if err != nil {
				panic(err)
			}

			m.t.SyncClient.MustPublish(ctx, MinerSyncTopic, &MiningSyncMsg{
				Addr:     m.Addresses[AddressMinerActor],
				Produced: produced,
			})

		case s := <-signalCh:
			// this is a signal that a miner is done mining.
			done = append(done, s.Addr)
			if s.Produced {
				produced[s.Addr] = struct{}{}
			}

			if len(done) != minerCnt {
				// still waiting for other miners.
				continue
			}

			// we've now ascertained that all miners have reported.
			//
			// if this is a null round, wait for no HEAD changes.
			// otherwise, we wait for blocks to propagate and for our HEAD
			// to progress to a tipset that contains the blocks from all
			// miners that produced a block, at the chain height we expect.
			if len(produced) > 0 {
				err := m.waitForHeadUpdate(ctx, epoch, produced)
				if err != nil {
					panic(err)
				}
			}

			// reset state in preparation for next round.
			done = done[0:0]
			for k := range produced {
				delete(produced, k)
			}

			// signal epoch end and loop over.
			mineDone <- epoch

		case <-ctx.Done():
			return

		case <-sub.Done():
			panic("mining synchronization subscription died")
		}
	}
}

func (m *LotusMiner) waitForHeadUpdate(ctx context.Context, epoch abi.ChainEpoch, blockProducers map[address.Address]struct{}) error {
	ctx, cancel := context.WithCancel(context.Background())
	changes := m.ChainStore.SubHeadChanges(ctx)
	defer cancel()

	for {
		select {
		case change := <-changes:
		Change:
			for _, c := range change {
				if c.Val.Height() != epoch {
					continue // not the epoch we're seeking.
				}
				blocks := c.Val.Blocks()
				if len(blocks) != len(blockProducers) {
					continue // this tipset still doesn't include all mined blocks.
				}
				for _, b := range blocks {
					if _, ok := blockProducers[b.Miner]; !ok {
						continue Change // not the HEAD we're looking for.
					}
				}
				// the HEAD tipset includes all blocks mined in this round; we're done!
				return nil
			}

		case <-ctx.Done():
			return fmt.Errorf("synchronizing mining: gave up while awaiting expected head update for epoch %d: %w", epoch, ctx.Err())
		}
	}
}

// mineRound attempts to mine a round. Apparently mining can fail sporadically:
// https://github.com/filecoin-project/lotus/pull/2259, so we perform 100
// attempts before giving up entirely.
func (m *LotusMiner) mineRound(ctx context.Context, epoch abi.ChainEpoch) (produced bool, err error) {
	const retries = 100
	for attempt := 0; attempt < retries; attempt++ {
		var (
			produced bool
			err      error
			done     = make(chan struct{})
		)

		err = m.MineOne(ctx, func(m bool, e error) {
			produced, err = m, e
			close(done)
		})
		if err != nil {
			// this is a serious error, we didn't even attempt to mine a block; abort.
			return false, fmt.Errorf("failed to trigger mining a block: %w", err)
		}
		<-done

		if err != nil {
			// mining failed.
			m.t.D().Counter("block.mine.err").Inc(1)
			m.t.RecordMessage("retrying block at height %d after %d attempts due to mining error: %s", epoch, attempt, err)
			continue
		}

		// all ok, mining completed.
		m.t.RecordMessage("~~~ mining completed! did we produce a block at epoch %d? %t ~~~", epoch, produced)
		if produced {
			m.t.D().Counter(fmt.Sprintf("block.mine,miner=%s", m.Addresses[AddressMinerActor])).Inc(1)
		}
		return produced, nil
	}

	return false, fmt.Errorf("failed to mine a block after %d retries", retries)
}
