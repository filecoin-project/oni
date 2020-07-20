
import os
import json
import pandas as pd
import hvplot.pandas # noqa - for side effects, don't remove
import panel as pn

from typing import List, Optional, NamedTuple

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


class ChainDataFrames(NamedTuple):
    miner_states: pd.DataFrame
    tipsets: pd.DataFrame

    def to_pickle(self, dir_path: str):
        self.miner_states.to_pickle(os.path.join(dir_path, 'miner_states.gz'))
        self.tipsets.to_pickle(os.path.join(dir_path, 'tipsets.gz'))

    @classmethod
    def read_pickle(cls, dir_path: str) -> 'ChainDataFrames':
        d = dict()
        for table in ['miner_states', 'tipsets']:
            p = os.path.join(dir_path, '{}.gz'.format(table))
            if not os.path.exists(p):
                raise ValueError('no file found at ' + p)
            d[table] = pd.read_pickle(p)
        return ChainDataFrames(**d)


    @classmethod
    def from_ndjson_file(cls, statefile: str) -> 'ChainDataFrames':
        miner_states = None
        tipsets = None

        with open(statefile, 'rt') as f:
            for line in f.readlines():
                j = json.loads(line)
                chain_height = j['Height']
                t = j.get('Tipset', None)
                if t is not None:
                    df = pd.json_normalize(t)
                    if tipsets is None:
                        tipsets = df
                    else:
                        tipsets = tipsets.append(df, ignore_index=True)

                miners = j['MinerStates']
                for m in miners.values():
                    df = pd.json_normalize(m)
                    df['Height'] = chain_height
                    df.rename(columns=MINER_STATE_COL_RENAMES, inplace=True)
                    if miner_states is None:
                        miner_states = df
                    else:
                        miner_states = miner_states.append(df, ignore_index=True)
        miner_states.fillna(0, inplace=True)
        miner_states.set_index('Height', inplace=True)

        for c in ATTO_FIL_COLS:
            miner_states[c] = miner_states[c].apply(atto_to_fil)

        for c in NUMERIC_COLS:
            miner_states[c] = miner_states[c].apply(pd.to_numeric)

        # the Sectors.* fields are lists of sector ids, but we want to plot counts, so
        # we pull the length of each list into a new column
        def count(x):
            if isinstance(x, int):
                return x
            return len(x)
        miner_states['CommittedSectors'] = miner_states['Sectors.Committed'].apply(count)
        miner_states['ProvingSectors'] = miner_states['Sectors.Proving'].apply(count)
        return ChainDataFrames(miner_states=miner_states, tipsets=tipsets)


def tick_formatter(col: str) -> Optional[str]:
    if col in ATTO_FIL_COLS:
        return '%8.4f FIL'
    return None


class ChainState(object):
    def __init__(self, pandas_data: ChainDataFrames):
        self.pandas = pandas_data

    @classmethod
    def from_ndjson(cls, filename: str):
        return ChainState(ChainDataFrames.from_ndjson_file(filename))

    @classmethod
    def from_pickle(cls, dir_path: str) -> 'ChainState':
        return ChainState(ChainDataFrames.read_pickle(dir_path))

    def to_pickle(self, dir_path):
        self.pandas.to_pickle(dir_path)

    def line_chart_selector_panel(self, variables: List[str] = None):
        if variables is None:
            variables = NUMERIC_COLS + DERIVED_COLS
        selector = pn.widgets.Select(name='Variable', options=variables)
        cols = ['Miner'] + variables
        plot = self.pandas.miner_states[cols].hvplot(by='Miner', y=selector)
        return pn.Column(pn.WidgetBox(selector), plot)

    def line_chart_stack(self, variables: List[str] = None):
        if variables is None:
            variables = NUMERIC_COLS + DERIVED_COLS

        plots = []
        for c in variables:
            title = c.split('.')[-1]
            p = self.pandas.miner_states[['Miner', c]].hvplot(by='Miner', y=c, title=title, yformatter=tick_formatter(c))
            plots.append(p)
        return pn.Column(*plots)

    def stacked_area(self, variable: str = 'Info.MinerPowerRaw', title: str = 'Miner Power Distribution (Raw)'):
        df = self.pandas.miner_states[['Miner', variable]]
        df = df.pivot_table(values=[variable], index=df.index, columns='Miner', aggfunc='sum')
        df = df.div(df.sum(1), axis=0)
        df.columns = df.columns.get_level_values(1)
        return df.hvplot.area(title=title)
