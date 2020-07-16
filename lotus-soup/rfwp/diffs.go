package rfwp

import (
	"fmt"

	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"
)

var (
	prevHeight = abi.ChainEpoch(-1)
	diffHeight = make(map[string]map[string]map[abi.ChainEpoch]big.Int)   // height -> value
	diffValue  = make(map[string]map[string]map[big.Int][]abi.ChainEpoch) // value -> []height
	diffCmp    = make(map[string]map[string]map[big.Int][]abi.ChainEpoch) // difference (height, height-1) -> []height
	valueTypes = []string{"MinerPower", "CommittedBytes", "ProvingBytes", "FaultyBytes", "Balance", "PreCommitDeposits", "LockedFunds", "AvailableFunds", "WorkerBalance", "MarketEscrow", "MarketLocked"}
)

func printDiff(mi *MinerInfo) {
	maddr := mi.MinerAddr.String()

	for valueName, mp := range diffCmp[maddr] {
		fmt.Println("=====", valueName, "=====")
		fmt.Println("=====len(", len(mp), ")=====")
		for difference, heights := range mp {
			fmt.Printf("diff of %v at heights %v\n", difference, heights)
		}
	}
}

func recordDiff(mi *MinerInfo, height abi.ChainEpoch) {
	maddr := mi.MinerAddr.String()
	if _, ok := diffHeight[maddr]; !ok {
		diffHeight[maddr] = make(map[string]map[abi.ChainEpoch]big.Int)
		diffValue[maddr] = make(map[string]map[big.Int][]abi.ChainEpoch)

		for _, v := range valueTypes {
			diffHeight[maddr][v] = make(map[abi.ChainEpoch]big.Int)
			diffValue[maddr][v] = make(map[big.Int][]abi.ChainEpoch)
		}
	}

	{
		value := big.Int(mi.MinerPower.MinerPower.RawBytePower)
		diffHeight[maddr]["MinerPower"][height] = value
		diffValue[maddr]["MinerPower"][value] = append(diffValue[maddr]["MinerPower"][value], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["MinerPower"][prevHeight]
			var cmp big.Int
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			diffCmp[maddr]["MinerPower"][cmp] = append(diffValue[maddr]["MinerPower"][cmp], height)
		}
	}

	{
		value := big.Int(mi.CommittedBytes)
		diffHeight[maddr]["CommittedBytes"][height] = value
		diffValue[maddr]["CommittedBytes"][value] = append(diffValue[maddr]["CommittedBytes"][value], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["CommittedBytes"][prevHeight]
			var cmp big.Int
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			diffCmp[maddr]["CommittedBytes"][cmp] = append(diffValue[maddr]["CommittedBytes"][cmp], height)
		}
	}

	{
		value := big.Int(mi.ProvingBytes)
		diffHeight[maddr]["ProvingBytes"][height] = value
		diffValue[maddr]["ProvingBytes"][value] = append(diffValue[maddr]["ProvingBytes"][value], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["ProvingBytes"][prevHeight]
			var cmp big.Int
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			diffCmp[maddr]["ProvingBytes"][cmp] = append(diffValue[maddr]["ProvingBytes"][cmp], height)
		}
	}

	{
		value := big.Int(mi.FaultyBytes)
		diffHeight[maddr]["FaultyBytes"][height] = value
		diffValue[maddr]["FaultyBytes"][value] = append(diffValue[maddr]["FaultyBytes"][value], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["FaultyBytes"][prevHeight]
			var cmp big.Int
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			diffCmp[maddr]["FaultyBytes"][cmp] = append(diffValue[maddr]["FaultyBytes"][cmp], height)
		}
	}

	{
		value := big.Int(mi.Balance)
		diffHeight[maddr]["Balance"][height] = value
		diffValue[maddr]["Balance"][value] = append(diffValue[maddr]["Balance"][value], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["Balance"][prevHeight]
			var cmp big.Int
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			diffCmp[maddr]["Balance"][cmp] = append(diffValue[maddr]["Balance"][cmp], height)
		}
	}

	{
		value := big.Int(mi.PreCommitDeposits)
		diffHeight[maddr]["PreCommitDeposits"][height] = value
		diffValue[maddr]["PreCommitDeposits"][value] = append(diffValue[maddr]["PreCommitDeposits"][value], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["PreCommitDeposits"][prevHeight]
			var cmp big.Int
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			diffCmp[maddr]["PreCommitDeposits"][cmp] = append(diffValue[maddr]["PreCommitDeposits"][cmp], height)
		}
	}

	{
		value := big.Int(mi.LockedFunds)
		diffHeight[maddr]["LockedFunds"][height] = value
		diffValue[maddr]["LockedFunds"][value] = append(diffValue[maddr]["LockedFunds"][value], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["LockedFunds"][prevHeight]
			var cmp big.Int
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			diffCmp[maddr]["LockedFunds"][cmp] = append(diffValue[maddr]["LockedFunds"][cmp], height)
		}
	}

	{
		value := big.Int(mi.AvailableFunds)
		diffHeight[maddr]["AvailableFunds"][height] = value
		diffValue[maddr]["AvailableFunds"][value] = append(diffValue[maddr]["AvailableFunds"][value], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["AvailableFunds"][prevHeight]
			var cmp big.Int
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			diffCmp[maddr]["AvailableFunds"][cmp] = append(diffValue[maddr]["AvailableFunds"][cmp], height)
		}
	}

	{
		value := big.Int(mi.WorkerBalance)
		diffHeight[maddr]["WorkerBalance"][height] = value
		diffValue[maddr]["WorkerBalance"][value] = append(diffValue[maddr]["WorkerBalance"][value], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["WorkerBalance"][prevHeight]
			var cmp big.Int
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			diffCmp[maddr]["WorkerBalance"][cmp] = append(diffValue[maddr]["WorkerBalance"][cmp], height)
		}
	}

	{
		value := big.Int(mi.MarketEscrow)
		diffHeight[maddr]["MarketEscrow"][height] = value
		diffValue[maddr]["MarketEscrow"][value] = append(diffValue[maddr]["MarketEscrow"][value], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["MarketEscrow"][prevHeight]
			var cmp big.Int
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			diffCmp[maddr]["MarketEscrow"][cmp] = append(diffValue[maddr]["MarketEscrow"][cmp], height)
		}
	}

	{
		value := big.Int(mi.MarketLocked)
		diffHeight[maddr]["MarketLocked"][height] = value
		diffValue[maddr]["MarketLocked"][value] = append(diffValue[maddr]["MarketLocked"][value], height)

		if prevHeight != -1 {
			prevValue := diffHeight[maddr]["MarketLocked"][prevHeight]
			var cmp big.Int
			cmp.Sub(value.Int, prevValue.Int) // value - prevValue
			diffCmp[maddr]["MarketLocked"][cmp] = append(diffValue[maddr]["MarketLocked"][cmp], height)
		}
	}

	prevHeight = height
}
