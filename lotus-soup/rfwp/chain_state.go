package rfwp

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
	"github.com/filecoin-project/lotus/lib/adtutil"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/apibstore"
	"github.com/filecoin-project/lotus/chain/types"

	"github.com/filecoin-project/oni/lotus-soup/testkit"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/builtin/miner"
	"github.com/filecoin-project/specs-actors/actors/builtin/power"
	sealing "github.com/filecoin-project/storage-fsm"

	rlepluslazy "github.com/filecoin-project/go-bitfield/rle"
	cbor "github.com/ipfs/go-ipld-cbor"

	tstats "github.com/filecoin-project/lotus/tools/stats"
)

func ChainState(t *testkit.TestEnvironment, m *testkit.LotusMiner) error {
	height := 0
	headlag := 3

	ctx := context.Background()

	tipsetsCh, err := tstats.GetTips(ctx, m.FullApi, abi.ChainEpoch(height), headlag)
	if err != nil {
		return err
	}

	jsonFilename := fmt.Sprintf("%s%cchain-state.ndjson", t.TestOutputsPath, os.PathSeparator)
	jsonFile, err := os.Create(jsonFilename)
	if err != nil {
		return err
	}
	defer jsonFile.Close()
	jsonEncoder := json.NewEncoder(jsonFile)

	for tipset := range tipsetsCh {
		maddrs, err := m.FullApi.StateListMiners(ctx, tipset.Key())
		if err != nil {
			return err
		}

		snapshot := ChainSnapshot{
			Height:      tipset.Height(),
			MinerStates: make(map[string]*MinerStateSnapshot),
		}

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

				minerInfo, err := info(t, m, maddr, w, tipset.Height())
				if err != nil {
					return err
				}
				writeText(w, minerInfo)

				faultState, err := provingFaults(t, m, maddr, tipset.Height())
				if err != nil {
					return err
				}
				writeText(w, faultState)

				provState, err := provingInfo(t, m, maddr, tipset.Height())
				if err != nil {
					return err
				}
				writeText(w, provState)

				deadlines, err := provingDeadlines(t, m, maddr, tipset.Height())
				if err != nil {
					return err
				}
				writeText(w, deadlines)

				sectorInfo, err := sectorsList(t, m, maddr, w, tipset.Height())
				if err != nil {
					return err
				}
				writeText(w, sectorInfo)

				snapshot.MinerStates[maddr.String()] = &MinerStateSnapshot{
					Info:        minerInfo,
					Faults:      faultState,
					ProvingInfo: provState,
					Deadlines:   deadlines,
					Sectors:     sectorInfo,
				}

				return jsonEncoder.Encode(snapshot)
			}()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

type ChainSnapshot struct {
	Height abi.ChainEpoch

	MinerStates map[string]*MinerStateSnapshot
}

type MinerStateSnapshot struct {
	Info *MinerInfo
	Faults *ProvingFaultState
	ProvingInfo *ProvingInfoState
	Deadlines *ProvingDeadlines
	Sectors *SectorInfo
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

func provingFaults(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, height abi.ChainEpoch) (*ProvingFaultState, error) {
	api := m.FullApi
	ctx := context.Background()

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
	faults, err := mas.Faults.All(100000000000)
	if err != nil {
		return nil, err
	}

	s := ProvingFaultState{FaultedSectors: make(map[int][]uint64)}

	if len(faults) == 0 {
		return &s, nil
	}
	head, err := api.ChainHead(ctx)
	if err != nil {
		return nil, err
	}
	deadlines, err := api.StateMinerDeadlines(ctx, maddr, head.Key())
	if err != nil {
		return nil, err
	}

	for deadline, sectors := range deadlines.Due {
		intersectSectors, _ := bitfield.IntersectBitField(sectors, mas.Faults)
		if intersectSectors != nil {
			allSectors, _ := intersectSectors.All(100000000000)
			for _, num := range allSectors {
				s.FaultedSectors[deadline] = append(s.FaultedSectors[deadline], num)
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
	NewSectors    uint64

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
	fmt.Fprintf(w, "New Sectors: %d\n\n", s.NewSectors)

	fmt.Fprintf(w, "Deadline Index:       %d\n", s.DeadlineIndex)
	fmt.Fprintf(w, "Deadline Sectors:     %d\n", s.DeadlineSectors)

	fmt.Fprintf(w, "Deadline Open:        %s\n", epochTime(s.CurrentEpoch, s.DeadlineOpen))
	fmt.Fprintf(w, "Deadline Close:       %s\n", epochTime(s.CurrentEpoch, s.DeadlineClose))
	fmt.Fprintf(w, "Deadline Challenge:   %s\n", epochTime(s.CurrentEpoch, s.DeadlineChallenge))
	fmt.Fprintf(w, "Deadline FaultCutoff: %s\n", epochTime(s.CurrentEpoch, s.DeadlineFaultCutoff))

	return w.Bytes(), nil
}

func provingInfo(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, height abi.ChainEpoch) (*ProvingInfoState, error) {
	api := m.FullApi
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

	newSectors, err := mas.NewSectors.Count()
	if err != nil {
		return nil, err
	}

	faults, err := mas.Faults.Count()
	if err != nil {
		return nil, err
	}

	recoveries, err := mas.Recoveries.Count()
	if err != nil {
		return nil, err
	}

	var provenSectors uint64
	for _, d := range deadlines.Due {
		c, err := d.Count()
		if err != nil {
			return nil, err
		}
		provenSectors += c
	}

	var faultPerc float64
	if provenSectors > 0 {
		faultPerc = float64(faults*10000/provenSectors) / 100
	}

	s := ProvingInfoState{
		CurrentEpoch:        cd.CurrentEpoch,
		ProvingPeriodStart:  cd.PeriodStart,
		Faults:              faults,
		ProvenSectors:       provenSectors,
		FaultPercent:        faultPerc,
		Recoveries:          recoveries,
		NewSectors:          newSectors,
		DeadlineIndex:       cd.Index,
		DeadlineOpen:        cd.Open,
		DeadlineClose:       cd.Close,
		DeadlineChallenge:   cd.Challenge,
		DeadlineFaultCutoff: cd.FaultCutoff,
		WPoStProvingPeriod:  miner.WPoStProvingPeriod,
	}
	if cd.Index < uint64(len(deadlines.Due)) {
		curDeadlineSectors, err := deadlines.Due[cd.Index].Count()
		if err != nil {
			return nil, err
		}
		s.DeadlineSectors = curDeadlineSectors
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
	Sectors uint64
	Partitions uint64
	Proven uint64
	Current bool
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

func provingDeadlines(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, height abi.ChainEpoch) (*ProvingDeadlines, error) {
	api := m.FullApi
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
	var info *miner.MinerInfo
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

		info, err = mas.GetInfo(adtutil.NewStore(ctx, cbor.NewCborStore(apibstore.NewAPIBlockstore(api))))
		if err != nil {
			return nil, err
		}
	}

	infos := make([]DeadlineInfo, len(deadlines.Due))
	for i, field := range deadlines.Due {
		c, err := field.Count()
		if err != nil {
			return nil, err
		}

		firstPartition, sectorCount, err := miner.PartitionsForDeadline(deadlines, info.WindowPoStPartitionSectors, uint64(i))
		if err != nil {
			return nil, err
		}

		partitionCount := (sectorCount + info.WindowPoStPartitionSectors - 1) / info.WindowPoStPartitionSectors

		var provenPartitions uint64
		{
			var maskRuns []rlepluslazy.Run
			if firstPartition > 0 {
				maskRuns = append(maskRuns, rlepluslazy.Run{
					Val: false,
					Len: firstPartition,
				})
			}
			maskRuns = append(maskRuns, rlepluslazy.Run{
				Val: true,
				Len: partitionCount,
			})

			ppbm, err := bitfield.NewFromIter(&rlepluslazy.RunSliceIterator{Runs: maskRuns})
			if err != nil {
				return nil, err
			}

			pp, err := bitfield.IntersectBitField(ppbm, mas.PostSubmissions)
			if err != nil {
				return nil, err
			}

			provenPartitions, err = pp.Count()
			if err != nil {
				return nil, err
			}
		}

		outInfo := DeadlineInfo{
			Sectors:    c,
			Partitions: partitionCount,
			Proven:     provenPartitions,
			Current:    di.Index == uint64(i),
		}
		infos = append(infos, outInfo)
	}

	return &ProvingDeadlines{Deadlines: infos}, nil
}

type SectorInfo struct {
	Sectors []abi.SectorNumber
	SectorStates map[abi.SectorNumber]api.SectorInfo
	Committed []abi.SectorNumber
	Proving []abi.SectorNumber
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

func sectorsList(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, w io.Writer, height abi.ChainEpoch) (*SectorInfo, error) {
	node := m.FullApi
	ctx := context.Background()

	list, err := m.MinerApi.SectorsList(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i] < list[j]
	})

	i := SectorInfo{Sectors: list, SectorStates: make(map[abi.SectorNumber]api.SectorInfo, len(list))}

	pset, err := node.StateMinerProvingSet(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	for _, info := range pset {
		i.Proving = append(i.Proving, info.ID)
	}

	sset, err := node.StateMinerSectors(ctx, maddr, nil, true, types.EmptyTSK)
	if err != nil {
		return nil, err
	}
	for _, info := range sset {
		i.Committed = append(i.Committed, info.ID)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i] < list[j]
	})

	for _, s := range list {
		st, err := m.MinerApi.SectorsStatus(ctx, s)
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

			fmt.Print("Expected block win rate: ")
			fmt.Fprintf(w, "%.4f/day (every %s)\n", winPerDay, winRate.Truncate(time.Second))
		}
	}

	fmt.Println()
	fmt.Fprintf(w, "Miner Balance: %s\n", types.FIL(i.Balance))
	fmt.Fprintf(w, "\tPreCommit:   %s\n", types.FIL(i.PreCommitDeposits))
	fmt.Fprintf(w, "\tLocked:      %s\n", types.FIL(i.LockedFunds))
	fmt.Fprintf(w, "\tAvailable:   %s\n", types.FIL(i.AvailableFunds))
	fmt.Fprintf(w, "Worker Balance: %s\n", types.FIL(i.WorkerBalance))
	fmt.Fprintf(w, "Market (Escrow):  %s\n", types.FIL(i.MarketEscrow))
	fmt.Fprintf(w, "Market (Locked):  %s\n\n", types.FIL(i.MarketLocked))

	fmt.Println("Sectors:")
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

func info(t *testkit.TestEnvironment, m *testkit.LotusMiner, maddr address.Address, w io.Writer, height abi.ChainEpoch) (*MinerInfo, error) {
	api := m.FullApi
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

	i.CommittedBytes = types.BigMul(types.NewInt(secCounts.Sset), types.NewInt(uint64(mi.SectorSize)))
	i.ProvingBytes = types.BigMul(types.NewInt(secCounts.Pset), types.NewInt(uint64(mi.SectorSize)))

	if nfaults != 0 {
		if secCounts.Sset != 0 {
			i.FaultyPercentage = float64(10000*nfaults/secCounts.Sset) / 100.
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

	sectors, err := m.MinerApi.SectorsList(ctx)
	if err != nil {
		return nil, err
	}

	buckets := map[sealing.SectorState]int{
		"Total": len(sectors),
	}
	for _, s := range sectors {
		st, err := m.MinerApi.SectorsStatus(ctx, s)
		if err != nil {
			return nil, err
		}

		buckets[sealing.SectorState(st.State)]++
	}
	i.SectorStateCounts = buckets

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
