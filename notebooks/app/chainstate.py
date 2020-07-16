
import json
import pandas as pd
import matplotlib.pyplot as plt
import hvplot.pandas # for side effects, don't remove
import panel as pn

from typing import List

MINER_STATE_COL_RENAMES = {
    'Info.MinerAddr': 'Miner',
    'Info.MinerPower.MinerPower.RawBytePower': 'Info.MinerPowerRaw',
    'Info.MinerPower.MinerPower.QualityAdjPower': 'Info.MinerPowerQualityAdj',
    'Info.MinerPower.TotalPower.RawBytePower': 'Info.TotalPowerRaw',
    'Info.MinerPower.TotalPower.QualityAdjPower': 'Info.TotalPowerQualityAdj',
}

# columns that we need to convert to numeric types from stringified big.Ints
NUMERIC_COLS = [
    'Info.MinerPowerRaw',
    'Info.MinerPowerQualityAdj',
    'Info.TotalPowerRaw',
    'Info.TotalPowerQualityAdj',
    'Info.Balance',
    'Info.CommittedBytes',
    'Info.ProvingBytes',
    'Info.FaultyBytes',
    'Info.FaultyPercentage',
    'Info.PreCommitDeposits',
    'Info.LockedFunds',
    'Info.AvailableFunds',
    'Info.WorkerBalance',
    'Info.MarketEscrow',
    'Info.MarketLocked',
]

DERIVED_COLS = [
    'CommittedSectors',
    'ProvingSectors',
]

ATTO_FIL_COLS = [
    'Info.Balance',
    'Info.PreCommitDeposits',
    'Info.LockedFunds',
    'Info.AvailableFunds',
    'Info.WorkerBalance',
    'Info.MarketEscrow',
    'Info.MarketLocked',
]

def atto_to_fil(x):
    return float(x) * pow(10, -18)

def chain_state_to_pandas(statefile: str) -> pd.DataFrame:
    chain = None

    with open(statefile, 'rt') as f:
        for line in f.readlines():
            j = json.loads(line)
            chain_height = j['Height']

            miners = j['MinerStates']
            for m in miners.values():
                df = pd.json_normalize(m)
                df['Height'] = chain_height
                df.rename(columns=MINER_STATE_COL_RENAMES, inplace=True)
                if chain is None:
                    chain = df
                else:
                    chain = chain.append(df, ignore_index=True)
    chain.fillna(0, inplace=True)
    chain.set_index('Height', inplace=True)

    for c in ATTO_FIL_COLS:
        chain[c] = chain[c].apply(atto_to_fil)

    for c in NUMERIC_COLS:
        chain[c] = chain[c].apply(pd.to_numeric)

    # the Sectors.* fields are lists of sector ids, but we want to plot counts, so
    # we pull the length of each list into a new column
    chain['CommittedSectors'] = chain['Sectors.Committed'].apply(lambda x: len(x))
    chain['ProvingSectors'] = chain['Sectors.Proving'].apply(lambda x: len(x))
    return chain


class ChainState(object):
    def __init__(self, state_dataframe: pd.DataFrame):
        self.df = state_dataframe

    @classmethod
    def from_ndjson(cls, filename: str):
        return ChainState(chain_state_to_pandas(filename))

    def line_chart_selector_panel(self, variables: List[str] = None):
        if variables is None:
            variables = NUMERIC_COLS + DERIVED_COLS
        selector = pn.widgets.Select(name='Variable', options=variables)
        cols = ['Miner'] + variables
        plot = self.df[cols].hvplot(by='Miner', y=selector)
        return pn.Column(pn.WidgetBox(selector), plot)

    def line_chart_stack(self, variables: List[str] = None):
        if variables is None:
            variables = NUMERIC_COLS + DERIVED_COLS

        plots = []
        for c in variables:
            title = c.split('.')[-1]
            p = self.df[['Miner', c]].hvplot(by='Miner', y=c, title=title)
            plots.append(p)
        return pn.Column(*plots)

    def stacked_area(self, variable: str = 'Info.MinerPowerRaw', title: str = 'Miner Power Distribution (Raw)'):
        df = self.df[['Miner', variable]]
        df = df.pivot_table(values=[variable], index=df.index, columns='Miner', aggfunc='sum')
        df = df.div(df.sum(1), axis=0)
        df.columns = df.columns.get_level_values(1)
        return df.hvplot.area(title=title)
