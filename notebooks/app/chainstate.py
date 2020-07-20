
import os
import json
import pandas as pd
import hvplot.pandas # noqa - for side effects, don't remove
import panel as pn

from typing import List, Optional, NamedTuple, Any

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


class ChainDataFrames(object):

    # miner_states only includes state snapshots for tipsets that were included in the final chain
    miner_states: pd.DataFrame

    # transient_miner_states has state snapshots that were not included in the final chain (tipset reverted)
    transient_miner_states: pd.DataFrame

    # all_miner_states has all state snapshots, whether included in the final chain or not
    all_miner_states: pd.DataFrame

    # tipsets has all tipsets, whether included in the final chain or not
    tipsets: pd.DataFrame

    # head_changes has a sequence of apply/revert operations that result in the final chain
    head_changes: pd.DataFrame

    def __init__(self, all_miner_states: pd.DataFrame, tipsets: pd.DataFrame, head_changes: pd.DataFrame):
        # annotate each tipset with whether it was included in the "final" chain
        # we determine this by summing up the "apply"/"revert" commands that have the same TipsetKey
        # using +1 for apply and -1 for revert. Any tipset with a positive score is considered included

        head_changes['apply_score'] = head_changes['Type'].apply(lambda t: -1 if t == 'revert' else 1)
        df = head_changes[['TipsetKey', 'apply_score']].groupby('TipsetKey').sum()

        joined = tipsets.join(df, on='TipsetKey')
        joined['included'] = joined['apply_score'] > 0
        tipsets['included'] = joined['included']

        joined = all_miner_states.join(df, on='TipsetKey')
        joined['included'] = joined['apply_score'] > 0
        all_miner_states['included'] = joined['included']

        # head_changes.drop(columns=['apply_score'], inplace=True)

        self.miner_states = all_miner_states.where(all_miner_states['included']).dropna()
        self.transient_miner_states = all_miner_states.where(all_miner_states['included'] != True).dropna()
        self.all_miner_states = all_miner_states
        self.tipsets = tipsets
        self.head_changes = head_changes

    def to_pickle(self, dir_path: str):
        self.all_miner_states.to_pickle(os.path.join(dir_path, 'all_miner_states.gz'))
        self.tipsets.to_pickle(os.path.join(dir_path, 'tipsets.gz'))
        self.head_changes.to_pickle(os.path.join(dir_path, 'head_changes.gz'))

    @classmethod
    def read_pickle(cls, dir_path: str) -> 'ChainDataFrames':
        d = dict()
        for table in ['all_miner_states', 'tipsets', 'head_changes']:
            p = os.path.join(dir_path, '{}.gz'.format(table))
            if not os.path.exists(p):
                raise ValueError('no file found at ' + p)
            d[table] = pd.read_pickle(p)
        return ChainDataFrames(**d)


    @classmethod
    def from_ndjson_files(cls, dir_path: str) -> 'ChainDataFrames':
        all_miner_states = cls.load_miner_state_ndjson(os.path.join(dir_path, 'chain-state.ndjson'))
        tipsets = cls.load_tipsets_ndjson(os.path.join(dir_path, 'chain-tipsets.ndjson'))
        head_changes = cls.load_head_changes_ndjson(os.path.join(dir_path, 'chain-head-changes.ndjson'))
        return ChainDataFrames(all_miner_states=all_miner_states, tipsets=tipsets, head_changes=head_changes)

    @classmethod
    def load_miner_state_ndjson(cls, statefile: str) -> pd.DataFrame:
        miner_states = None

        with open(statefile, 'rt') as f:
            for line in f.readlines():
                j = json.loads(line)
                chain_height = j['Height']
                tipset_key = j['TipsetKey']

                miners = j['MinerStates']
                for m in miners.values():
                    df = pd.json_normalize(m)
                    df['Height'] = chain_height
                    df['TipsetKey'] = tipset_key
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
        return miner_states

    @classmethod
    def load_tipsets_ndjson(cls, tipset_file: str) -> pd.DataFrame:
        tipsets = None
        with open(tipset_file, 'rt') as f:
            for line in f.readlines():
                j = json.loads(line)
                df = pd.json_normalize(j)
                if tipsets is None:
                    tipsets = df
                else:
                    tipsets = tipsets.append(df, ignore_index=True)
        return tipsets

    @classmethod
    def load_head_changes_ndjson(cls, head_change_file: str) -> pd.DataFrame:
        changes = None
        with open(head_change_file, 'rt') as f:
            for line in f.readlines():
                j = json.loads(line)
                df = pd.json_normalize(j)
                if changes is None:
                    changes = df
                else:
                    changes = changes.append(df, ignore_index=True)
        return changes


def tick_formatter(col: str) -> Optional[str]:
    if col in ATTO_FIL_COLS:
        return '%8.4f FIL'
    return None


class ChainState(object):
    def __init__(self, pandas_data: ChainDataFrames):
        self.pandas = pandas_data


    @classmethod
    def from_ndjson_files(cls, dir_path: str):
        return ChainState(ChainDataFrames.from_ndjson_files(dir_path))

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
