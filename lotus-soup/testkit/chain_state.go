package testkit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/lib/adtutil"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	sealing "github.com/filecoin-project/storage-fsm"

	cbor "github.com/ipfs/go-ipld-cbor"
)

func WaitForSlash(t *TestEnvironment, msg SlashedMinerMsg) chan error {
	// assert that balance got reduced with that much 5 times (sector fee)
	// assert that balance got reduced with that much 2 times (termination fee)
	// assert that balance got increased with that much 10 times (block reward)
	// assert that power got increased with that much 1 times (after sector is sealed)
	// assert that power got reduced with that much 1 times (after sector is announced faulty)
	slashedMiner := msg.MinerActorAddr

	errc := make(chan error)
	go func() {
		foundSlashConditions := false
		for range time.Tick(10 * time.Second) {
			if foundSlashConditions {
				close(errc)
				return
			}
			t.RecordMessage("wait for slashing, tick")
			func() {
				cs.Lock()
				defer cs.Unlock()

				negativeAmounts := []big.Int{}
				negativeDiffs := make(map[big.Int][]abi.ChainEpoch)

				for am, heights := range cs.DiffCmp[slashedMiner.String()]["LockedFunds"] {
					amount, err := big.FromString(am)
					if err != nil {
						errc <- fmt.Errorf("cannot parse LockedFunds amount: %w:", err)
						return
					}

					// amount is negative => slash condition
					if big.Cmp(amount, big.Zero()) < 0 {
						negativeDiffs[amount] = heights
						negativeAmounts = append(negativeAmounts, amount)
					}
				}

				t.RecordMessage("negative diffs: %d", len(negativeDiffs))
				if len(negativeDiffs) < 3 {
					return
				}

				sort.Slice(negativeAmounts, func(i, j int) bool { return big.Cmp(negativeAmounts[i], negativeAmounts[j]) > 0 })

				// TODO: confirm the largest is > 18 filecoin
				// TODO: confirm the next largest is > 9 filecoin
				foundSlashConditions = true
			}()
		}
	}()

	return errc
}

func UpdateChainState(t *TestEnvironment, n *LotusNode) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.RecordMessage("subscribing to chain notifications")
	notif, err := n.FullApi.ChainNotify(ctx)
	if err != nil {
		return err
	}
	// buffer the head changes to avoid blocking the notification channel
	// we also add a timestamp to get an approximation of when the change was
	// emitted
	changeCh := make(chan []*HeadChange, 4096)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case changes := <-notif:
				now := time.Now().UnixNano()
				out := make([]*HeadChange, len(changes))
				for i, c := range changes {
					out[i] = &HeadChange{
						ObservedAt: now,
						Type:       c.Type,
						TipsetKey:  c.Val.Key().String(),
						Height:     c.Val.Height(),
						tipset:     c.Val,
					}
				}
				changeCh <- out
			}
		}
	}()

	chainStateEnc, chainStateFile, err := makeJsonFile(t, "chain-state.ndjson")
	if err != nil {
		return err
	}
	defer chainStateFile.Close()

	headStateEnc, headChangeFile, err := makeJsonFile(t, "chain-head-changes.ndjson")
	if err != nil {
		return err
	}
	defer headChangeFile.Close()

	tipsetEnc, tipsetFile, err := makeJsonFile(t, "chain-tipsets.ndjson")
	if err != nil {
		return err
	}
	defer tipsetFile.Close()

	t.RecordMessage("created chain json files")

	recordHeadChange := func(change *HeadChange) error {
		return headStateEnc.Encode(change)
	}

	recordedTipsets := make(map[string]struct{})
	recordTipset := func(tipset *types.TipSet) error {
		if _, ok := recordedTipsets[tipset.Key().String()]; ok {
			return nil
		}
		return tipsetEnc.Encode(struct {
			TipsetKey string
			Tipset    *types.TipSet
		}{
			TipsetKey: tipset.Key().String(),
			Tipset:    tipset,
		})
	}

	recordedSnapshots := make(map[string]struct{})
	recordChainSnapshot := func(tipset *types.TipSet) error {
		tsk := tipset.Key()
		if _, ok := recordedSnapshots[tsk.String()]; ok {
			return nil
		}

		maddrs, err := n.FullApi.StateListMiners(ctx, tsk)
		if err != nil {
			return err
		}

		snapshot := ChainSnapshot{
			Height:      tipset.Height(),
			TipsetKey:   tsk.String(),
			MinerStates: make(map[string]*MinerStateSnapshot),
		}

		return func() error {
			cs.Lock()
			defer cs.Unlock()

			for _, maddr := range maddrs {
				err := func() error {
					filename := fmt.Sprintf("%s%cstate-%s-%d", t.TestOutputsPath, os.PathSeparator, maddr, tipset.Height())

					f, err := os.Create(filename)
					if err != nil {
						return err
					}
					defer f.Close()

					w := bufio.NewWriter(f)
					defer w.Flush()

					minerInfo, err := info(t, n, maddr, w, tipset.Height())
					if err != nil {
						return err
					}
					writeText(w, minerInfo)

					if tipset.Height()%100 == 0 {
						printDiff(t, minerInfo, tipset.Height())
					}

					faultState, err := provingFaults(t, n, maddr, tipset.Height())
					if err != nil {
						return err
					}
					writeText(w, faultState)

					provState, err := provingInfo(t, n, maddr, tipset.Height())
					if err != nil {
						return err
					}
					writeText(w, provState)

					// record diff
					recordDiff(minerInfo, provState, tipset.Height())

					deadlines, err := provingDeadlines(t, n, maddr, tipset.Height())
					if err != nil {
						return err
					}
					writeText(w, deadlines)

					sectorInfo, err := sectorsList(t, n, maddr, w, tipset.Height())
					if err != nil {
						return err
					}
					if sectorInfo != nil {
						writeText(w, sectorInfo)
					}

					snapshot.MinerStates[maddr.String()] = &MinerStateSnapshot{
						Info:        minerInfo,
						Faults:      faultState,
						ProvingInfo: provState,
						Deadlines:   deadlines,
						Sectors:     sectorInfo,
					}

					return chainStateEnc.Encode(snapshot)
				}()
				if err != nil {
					return err
				}
			}

			cs.PrevHeight = tipset.Height()
			return nil
		}()
	}

	for {
		select {
		case <-ctx.Done():
			break

		case changes := <-changeCh:
			for _, change := range changes {
				if err := recordHeadChange(change); err != nil {
					return err
				}
				switch change.Type {
				case store.HCCurrent:
					tipsets, err := loadTipsets(ctx, n.FullApi, change.tipset, 0)
					if err != nil {
						return err
					}

					for _, tipset := range tipsets {
						if err := recordTipset(tipset); err != nil {
							return err
						}
						if err := recordChainSnapshot(tipset); err != nil {
							return err
						}
					}
				case store.HCApply:
					if err := recordTipset(change.tipset); err != nil {
						return err
					}
					if err := recordChainSnapshot(change.tipset); err != nil {
						return err
					}
				}
			}
		}
	}
}

func makeJsonFile(t *TestEnvironment, filename string) (enc *json.Encoder, close io.Closer, err error) {
	filePath := fmt.Sprintf("%s%c%s", t.TestOutputsPath, os.PathSeparator, filename)
	f, err := os.Create(filePath)
	if err != nil {
		return nil, nil, err
	}
	return json.NewEncoder(f), f, err
}

func loadTipsets(ctx context.Context, api api.FullNode, curr *types.TipSet, lowestHeight abi.ChainEpoch) ([]*types.TipSet, error) {
	tipsets := []*types.TipSet{}
	for {
		if curr.Height() == 0 {
			break
		}

		if curr.Height() <= lowestHeight {
			break
		}

		tipsets = append(tipsets, curr)

		tsk := curr.Parents()
		prev, err := api.ChainGetTipSet(ctx, tsk)
		if err != nil {
			return tipsets, err
		}

		curr = prev
	}

	for i, j := 0, len(tipsets)-1; i < j; i, j = i+1, j-1 {
		tipsets[i], tipsets[j] = tipsets[j], tipsets[i]
	}

	return tipsets, nil
}

type HeadChange struct {
	Type       string
	TipsetKey  string
	Height     abi.ChainEpoch
	ObservedAt int64
	tipset     *types.TipSet
}

type ChainSnapshot struct {
	Height      abi.ChainEpoch
	TipsetKey   string
	MinerStates map[string]*MinerStateSnapshot
}

type MinerStateSnapshot struct {
	Info        *MinerInfo
	Faults      *ProvingFaultState
	ProvingInfo *ProvingInfoState
	Deadlines   *ProvingDeadlines
	Sectors     *SectorInfo
}

// writeText marshals m to text and writes to w, swallowing any errors along the way.
func writeText(w io.Writer, m plainTextMarshaler) {
	b, err := m.MarshalPlainText()
	if err != nil {
		return
	}
	_, _ = w.Write(b)
}

// if we make our structs `encoding.TextMarshaler`s, they all get stringified when marshaling to JSON
// instead of just using the default struct marshaler.
// so here's encoding.TextMarshaler with a different name, so that doesn't happen.
type plainTextMarshaler interface {
	MarshalPlainText() ([]byte, error)
}

type ProvingFaultState struct {
	// FaultedSectors is a map of deadline indices to a list of faulted sectors for that proving window.
	// If the miner has no faulty sectors, the map will be empty.
	FaultedSectors map[int][]uint64
}

func (s *ProvingFaultState) MarshalPlainText() ([]byte, error) {
	w := &bytes.Buffer{}

	if len(s.FaultedSectors) == 0 {
		fmt.Fprintf(w, "no faulty sectors\n")
		return w.Bytes(), nil
	}

	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "deadline\tsectors")
	for deadline := 0; deadline < int(miner.WPoStPeriodDeadlines); deadline++ {
		if sectors, ok := s.FaultedSectors[deadline]; ok {
			for _, num := range sectors {
				_, _ = fmt.Fprintf(tw, "%d\t%d\n", deadline, num)
			}
		}
	}

	return w.Bytes(), nil
}

func provingFaults(t *TestEnvironment, n *LotusNode, maddr address.Address, height abi.ChainEpoch) (*ProvingFaultState, error) {
	api := n.FullApi
	ctx := context.Background()

	s := ProvingFaultState{FaultedSectors: make(map[int][]uint64)}

	head, err := api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	deadlines, err := api.StateMinerDeadlines(ctx, maddr, head.Key())
	if err != nil {
		return nil, err
	}
	for dlIdx := range deadlines {
		partitions, err := api.StateMinerPartitions(ctx, maddr, uint64(dlIdx), types.EmptyTSK)
		if err != nil {
			return nil, err
		}

		for _, partition := range partitions {
			faulty, err := partition.Faults.All(10000000)
			if err != nil {
				return nil, err
			}

			for _, num := range faulty {
				s.FaultedSectors[dlIdx] = append(s.FaultedSectors[dlIdx], num)
			}
		}
	}

	return &s, nil
}

type ProvingInfoState struct {
	CurrentEpoch abi.ChainEpoch

	ProvingPeriodStart abi.ChainEpoch

	Faults        uint64
	ProvenSectors uint64
	FaultPercent  float64
	Recoveries    uint64

	DeadlineIndex       uint64
	DeadlineSectors     uint64
	DeadlineOpen        abi.ChainEpoch
	DeadlineClose       abi.ChainEpoch
	DeadlineChallenge   abi.ChainEpoch
	DeadlineFaultCutoff abi.ChainEpoch

	WPoStProvingPeriod abi.ChainEpoch
}

func (s *ProvingInfoState) MarshalPlainText() ([]byte, error) {
	w := &bytes.Buffer{}
	fmt.Fprintf(w, "Current Epoch:           %d\n", s.CurrentEpoch)
	fmt.Fprintf(w, "Chain Period:            %d\n", s.CurrentEpoch/s.WPoStProvingPeriod)
	fmt.Fprintf(w, "Chain Period Start:      %s\n", epochTime(s.CurrentEpoch, (s.CurrentEpoch/s.WPoStProvingPeriod)*s.WPoStProvingPeriod))
	fmt.Fprintf(w, "Chain Period End:        %s\n\n", epochTime(s.CurrentEpoch, (s.CurrentEpoch/s.WPoStProvingPeriod+1)*s.WPoStProvingPeriod))

	fmt.Fprintf(w, "Proving Period Boundary: %d\n", s.ProvingPeriodStart%s.WPoStProvingPeriod)
	fmt.Fprintf(w, "Proving Period Start:    %s\n", epochTime(s.CurrentEpoch, s.ProvingPeriodStart))
	fmt.Fprintf(w, "Next Period Start:       %s\n\n", epochTime(s.CurrentEpoch, s.ProvingPeriodStart+s.WPoStProvingPeriod))

	fmt.Fprintf(w, "Faults:      %d (%.2f%%)\n", s.Faults, s.FaultPercent)
	fmt.Fprintf(w, "Recovering:  %d\n", s.Recoveries)
	//fmt.Fprintf(w, "New Sectors: %d\n\n", s.NewSectors)

	fmt.Fprintf(w, "Deadline Index:       %d\n", s.DeadlineIndex)
	fmt.Fprintf(w, "Deadline Sectors:     %d\n", s.DeadlineSectors)

	fmt.Fprintf(w, "Deadline Open:        %s\n", epochTime(s.CurrentEpoch, s.DeadlineOpen))
	fmt.Fprintf(w, "Deadline Close:       %s\n", epochTime(s.CurrentEpoch, s.DeadlineClose))
	fmt.Fprintf(w, "Deadline Challenge:   %s\n", epochTime(s.CurrentEpoch, s.DeadlineChallenge))
	fmt.Fprintf(w, "Deadline FaultCutoff: %s\n", epochTime(s.CurrentEpoch, s.DeadlineFaultCutoff))

	return w.Bytes(), nil
}

func provingInfo(t *TestEnvironment, n *LotusNode, maddr address.Address, height abi.ChainEpoch) (*ProvingInfoState, error) {
	api := n.FullApi
	ctx := context.Background()

	head, err := api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}

	cd, err := api.StateMinerProvingDeadline(ctx, maddr, head.Key())
	if err != nil {
		return nil, err
	}

	deadlines, err := api.StateMinerDeadlines(ctx, maddr, head.Key())
	if err != nil {
		return nil, err
	}

	var mas miner.State
	{
		mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return nil, err
		}
		rmas, err := api.ChainReadObj(ctx, mact.Head)
		if err != nil {
			return nil, err
		}
		if err := mas.UnmarshalCBOR(bytes.NewReader(rmas)); err != nil {
			return nil, err
		}
	}

	parts := map[uint64][]*miner.Partition{}
	for dlIdx := range deadlines {
		part, err := api.StateMinerPartitions(ctx, maddr, uint64(dlIdx), types.EmptyTSK)
		if err != nil {
			return nil, err
		}

		parts[uint64(dlIdx)] = part
	}

	proving := uint64(0)
	faults := uint64(0)
	recovering := uint64(0)

	for _, partitions := range parts {
		for _, partition := range partitions {
			sc, err := partition.Sectors.Count()
			if err != nil {
				return nil, err
			}
			proving += sc

			fc, err := partition.Faults.Count()
			if err != nil {
				return nil, err
			}
			faults += fc

			rc, err := partition.Faults.Count()
			if err != nil {
				return nil, err
			}
			recovering += rc
		}
	}

	var faultPerc float64
	if proving > 0 {
		faultPerc = float64(faults*10000/proving) / 100
	}

	s := ProvingInfoState{
		CurrentEpoch:        cd.CurrentEpoch,
		ProvingPeriodStart:  cd.PeriodStart,
		Faults:              faults,
		ProvenSectors:       proving,
		FaultPercent:        faultPerc,
		Recoveries:          recovering,
		DeadlineIndex:       cd.Index,
		DeadlineOpen:        cd.Open,
		DeadlineClose:       cd.Close,
		DeadlineChallenge:   cd.Challenge,
		DeadlineFaultCutoff: cd.FaultCutoff,
		WPoStProvingPeriod:  miner.WPoStProvingPeriod,
	}

	if cd.Index < miner.WPoStPeriodDeadlines {
		for _, partition := range parts[cd.Index] {
			sc, err := partition.Sectors.Count()
			if err != nil {
				return nil, err
			}
			s.DeadlineSectors += sc
		}
	}

	return &s, nil
}

func epochTime(curr, e abi.ChainEpoch) string {
	switch {
	case curr > e:
		return fmt.Sprintf("%d (%s ago)", e, time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(curr-e)))
	case curr == e:
		return fmt.Sprintf("%d (now)", e)
	case curr < e:
		return fmt.Sprintf("%d (in %s)", e, time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(e-curr)))
	}

	panic("math broke")
}

type ProvingDeadlines struct {
	Deadlines []DeadlineInfo
}

type DeadlineInfo struct {
	Sectors    uint64
	Partitions int
	Proven     uint64
	Current    bool
}

func (d *ProvingDeadlines) MarshalPlainText() ([]byte, error) {
	w := new(bytes.Buffer)
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "deadline\tsectors\tpartitions\tproven")

	for i, di := range d.Deadlines {
		var cur string
		if di.Current {
			cur += "\t(current)"
		}
		_, _ = fmt.Fprintf(tw, "%d\t%d\t%d\t%d%s\n", i, di.Sectors, di.Partitions, di.Proven, cur)
	}
	tw.Flush()
	return w.Bytes(), nil
}

func provingDeadlines(t *TestEnvironment, n *LotusNode, maddr address.Address, height abi.ChainEpoch) (*ProvingDeadlines, error) {
	api := n.FullApi
	ctx := context.Background()

	deadlines, err := api.StateMinerDeadlines(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	di, err := api.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	var mas miner.State
	{
		mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return nil, err
		}
		rmas, err := api.ChainReadObj(ctx, mact.Head)
		if err != nil {
			return nil, err
		}
		if err := mas.UnmarshalCBOR(bytes.NewReader(rmas)); err != nil {
			return nil, err
		}

		_, err = mas.GetInfo(adtutil.NewStore(ctx, cbor.NewCborStore(apibstore.NewAPIBlockstore(api))))
		if err != nil {
			return nil, err
		}
	}

	infos := make([]DeadlineInfo, 0, len(deadlines))
	for dlIdx, deadline := range deadlines {
		partitions, err := api.StateMinerPartitions(ctx, maddr, uint64(dlIdx), types.EmptyTSK)
		if err != nil {
			return nil, err
		}

		provenPartitions, err := deadline.PostSubmissions.Count()
		if err != nil {
			return nil, err
		}

		var cur string
		if di.Index == uint64(dlIdx) {
			cur += "\t(current)"
		}

		outInfo := DeadlineInfo{
			//Sectors:    c,
			Partitions: len(partitions),
			Proven:     provenPartitions,
			Current:    di.Index == uint64(dlIdx),
		}
		infos = append(infos, outInfo)
		//_, _ = fmt.Fprintf(tw, "%d\t%d\t%d%s\n", dlIdx, len(partitions), provenPartitions, cur)
	}

	return &ProvingDeadlines{Deadlines: infos}, nil
}

type SectorInfo struct {
	Sectors      []abi.SectorNumber
	SectorStates map[abi.SectorNumber]api.SectorInfo
	Committed    []abi.SectorNumber
	Proving      []abi.SectorNumber
}

func (i *SectorInfo) MarshalPlainText() ([]byte, error) {
	provingIDs := make(map[abi.SectorNumber]struct{}, len(i.Proving))
	for _, id := range i.Proving {
		provingIDs[id] = struct{}{}
	}
	commitedIDs := make(map[abi.SectorNumber]struct{}, len(i.Committed))
	for _, id := range i.Committed {
		commitedIDs[id] = struct{}{}
	}

	w := new(bytes.Buffer)
	tw := tabwriter.NewWriter(w, 8, 4, 1, ' ', 0)

	for _, s := range i.Sectors {
		_, inSSet := commitedIDs[s]
		_, inPSet := provingIDs[s]

		st, ok := i.SectorStates[s]
		if !ok {
			continue
		}

		fmt.Fprintf(tw, "%d: %s\tsSet: %s\tpSet: %s\ttktH: %d\tseedH: %d\tdeals: %v\n",
			s,
			st.State,
			yesno(inSSet),
			yesno(inPSet),
			st.Ticket.Epoch,
			st.Seed.Epoch,
			st.Deals,
		)
	}

	if err := tw.Flush(); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

func sectorsList(t *TestEnvironment, n *LotusNode, maddr address.Address, w io.Writer, height abi.ChainEpoch) (*SectorInfo, error) {
	if n.MinerApi == nil {
		return nil, nil
	}

	fullApi := n.FullApi
	ctx := context.Background()

	list, err := n.MinerApi.SectorsList(ctx)
	if err != nil {
		return nil, err
	}

	activeSet, err := fullApi.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	activeIDs := make(map[abi.SectorNumber]struct{}, len(activeSet))
	for _, info := range activeSet {
		activeIDs[info.ID] = struct{}{}
	}

	sset, err := fullApi.StateMinerSectors(ctx, maddr, nil, true, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	commitedIDs := make(map[abi.SectorNumber]struct{}, len(activeSet))
	for _, info := range sset {
		commitedIDs[info.ID] = struct{}{}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i] < list[j]
	})

	i := SectorInfo{Sectors: list, SectorStates: make(map[abi.SectorNumber]api.SectorInfo, len(list))}

	for _, s := range list {
		st, err := n.MinerApi.SectorsStatus(ctx, s)
		if err != nil {
			fmt.Fprintf(w, "%d:\tError: %s\n", s, err)
			continue
		}
		i.SectorStates[s] = st
	}
	return &i, nil
}

func yesno(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

type MinerInfo struct {
	MinerAddr  address.Address
	SectorSize string

	MinerPower *api.MinerPower

	CommittedBytes   big.Int
	ProvingBytes     big.Int
	FaultyBytes      big.Int
	FaultyPercentage float64

	Balance           big.Int
	PreCommitDeposits big.Int
	LockedFunds       big.Int
	AvailableFunds    big.Int
	WorkerBalance     big.Int
	MarketEscrow      big.Int
	MarketLocked      big.Int

	SectorStateCounts map[sealing.SectorState]int
}

func (i *MinerInfo) MarshalPlainText() ([]byte, error) {
	w := new(bytes.Buffer)
	fmt.Fprintf(w, "Miner: %s\n", i.MinerAddr)
	fmt.Fprintf(w, "Sector Size: %s\n", i.SectorSize)

	pow := i.MinerPower
	rpercI := types.BigDiv(types.BigMul(pow.MinerPower.RawBytePower, types.NewInt(1000000)), pow.TotalPower.RawBytePower)
	qpercI := types.BigDiv(types.BigMul(pow.MinerPower.QualityAdjPower, types.NewInt(1000000)), pow.TotalPower.QualityAdjPower)

	fmt.Fprintf(w, "Byte Power:   %s / %s (%0.4f%%)\n",
		types.SizeStr(pow.MinerPower.RawBytePower),
		types.SizeStr(pow.TotalPower.RawBytePower),
		float64(rpercI.Int64())/10000)

	fmt.Fprintf(w, "Actual Power: %s / %s (%0.4f%%)\n",
		types.DeciStr(pow.MinerPower.QualityAdjPower),
		types.DeciStr(pow.TotalPower.QualityAdjPower),
		float64(qpercI.Int64())/10000)

	fmt.Fprintf(w, "\tCommitted: %s\n", types.SizeStr(i.CommittedBytes))

	if i.FaultyBytes.Int == nil || i.FaultyBytes.IsZero() {
		fmt.Fprintf(w, "\tProving: %s\n", types.SizeStr(i.ProvingBytes))
	} else {
		fmt.Fprintf(w, "\tProving: %s (%s Faulty, %.2f%%)\n",
			types.SizeStr(i.ProvingBytes),
			types.SizeStr(i.FaultyBytes),
			i.FaultyPercentage)
	}

	if i.MinerPower.MinerPower.RawBytePower.LessThan(power.ConsensusMinerMinPower) {
		fmt.Fprintf(w, "Below minimum power threshold, no blocks will be won\n")
	} else {
		expWinChance := float64(types.BigMul(qpercI, types.NewInt(build.BlocksPerEpoch)).Int64()) / 1000000
		if expWinChance > 0 {
			if expWinChance > 1 {
				expWinChance = 1
			}
			winRate := time.Duration(float64(time.Second*time.Duration(build.BlockDelaySecs)) / expWinChance)
			winPerDay := float64(time.Hour*24) / float64(winRate)

			fmt.Fprintln(w, "Expected block win rate: ")
			fmt.Fprintf(w, "%.4f/day (every %s)\n", winPerDay, winRate.Truncate(time.Second))
		}
	}

	fmt.Fprintf(w, "Miner Balance: %s\n", types.FIL(i.Balance))
	fmt.Fprintf(w, "\tPreCommit:   %s\n", types.FIL(i.PreCommitDeposits))
	fmt.Fprintf(w, "\tLocked:      %s\n", types.FIL(i.LockedFunds))
	fmt.Fprintf(w, "\tAvailable:   %s\n", types.FIL(i.AvailableFunds))
	fmt.Fprintf(w, "Worker Balance: %s\n", types.FIL(i.WorkerBalance))
	fmt.Fprintf(w, "Market (Escrow):  %s\n", types.FIL(i.MarketEscrow))
	fmt.Fprintf(w, "Market (Locked):  %s\n\n", types.FIL(i.MarketLocked))

	buckets := i.SectorStateCounts

	var sorted []stateMeta
	for state, i := range buckets {
		sorted = append(sorted, stateMeta{i: i, state: state})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return stateOrder[sorted[i].state].i < stateOrder[sorted[j].state].i
	})

	for _, s := range sorted {
		_, _ = fmt.Fprintf(w, "\t%s: %d\n", s.state, s.i)
	}

	return w.Bytes(), nil
}

func info(t *TestEnvironment, n *LotusNode, maddr address.Address, w io.Writer, height abi.ChainEpoch) (*MinerInfo, error) {
	api := n.FullApi
	ctx := context.Background()

	mact, err := api.StateGetActor(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	var mas miner.State
	{
		rmas, err := api.ChainReadObj(ctx, mact.Head)
		if err != nil {
			return nil, err
		}
		if err := mas.UnmarshalCBOR(bytes.NewReader(rmas)); err != nil {
			return nil, err
		}
	}

	i := MinerInfo{MinerAddr: maddr}

	// Sector size
	mi, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	i.SectorSize = types.SizeStr(types.NewInt(uint64(mi.SectorSize)))

	i.MinerPower, err = api.StateMinerPower(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	secCounts, err := api.StateMinerSectorCount(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	faults, err := api.StateMinerFaults(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	nfaults, err := faults.Count()
	if err != nil {
		return nil, err
	}

	i.CommittedBytes = types.BigMul(types.NewInt(secCounts.Sectors), types.NewInt(uint64(mi.SectorSize)))
	i.ProvingBytes = types.BigMul(types.NewInt(secCounts.Active), types.NewInt(uint64(mi.SectorSize)))

	if nfaults != 0 {
		if secCounts.Sectors != 0 {
			i.FaultyPercentage = float64(10000*nfaults/secCounts.Sectors) / 100.
		}
		i.FaultyBytes = types.BigMul(types.NewInt(nfaults), types.NewInt(uint64(mi.SectorSize)))
	}

	i.Balance = mact.Balance
	i.PreCommitDeposits = mas.PreCommitDeposits
	i.LockedFunds = mas.LockedFunds
	i.AvailableFunds = types.BigSub(mact.Balance, types.BigAdd(mas.LockedFunds, mas.PreCommitDeposits))

	wb, err := api.WalletBalance(ctx, mi.Worker)
	if err != nil {
		return nil, err
	}
	i.WorkerBalance = wb

	mb, err := api.StateMarketBalance(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	i.MarketEscrow = mb.Escrow
	i.MarketLocked = mb.Locked

	if n.MinerApi != nil {
		sectors, err := n.MinerApi.SectorsList(ctx)
		if err != nil {
			return nil, err
		}

		buckets := map[sealing.SectorState]int{
			"Total": len(sectors),
		}
		for _, s := range sectors {
			st, err := n.MinerApi.SectorsStatus(ctx, s)
			if err != nil {
				return nil, err
			}

			buckets[sealing.SectorState(st.State)]++
		}
		i.SectorStateCounts = buckets
	}

	return &i, nil
}

type stateMeta struct {
	i     int
	state sealing.SectorState
}

var stateOrder = map[sealing.SectorState]stateMeta{}
var stateList = []stateMeta{
	{state: "Total"},
	{state: sealing.Proving},

	{state: sealing.UndefinedSectorState},
	{state: sealing.Empty},
	{state: sealing.Packing},
	{state: sealing.PreCommit1},
	{state: sealing.PreCommit2},
	{state: sealing.PreCommitting},
	{state: sealing.PreCommitWait},
	{state: sealing.WaitSeed},
	{state: sealing.Committing},
	{state: sealing.CommitWait},
	{state: sealing.FinalizeSector},

	{state: sealing.FailedUnrecoverable},
	{state: sealing.SealPreCommit1Failed},
	{state: sealing.SealPreCommit2Failed},
	{state: sealing.PreCommitFailed},
	{state: sealing.ComputeProofFailed},
	{state: sealing.CommitFailed},
	{state: sealing.PackingFailed},
	{state: sealing.FinalizeFailed},
	{state: sealing.Faulty},
	{state: sealing.FaultReported},
	{state: sealing.FaultedFinal},
}

func init() {
	for i, state := range stateList {
		stateOrder[state.state] = stateMeta{
			i: i,
		}
	}
}
