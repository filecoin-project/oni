
import os
import json
import multiprocessing as mp
import pandas as pd
import holoviews as hv
import hvplot.pandas # noqa - for side effects, don't remove
import panel as pn
from typing import List, Optional, NamedTuple, Any, Union

from .util import archive_temp_dir, extract_archive, benchmark

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


def read_ndjson(filename: str):
    """
    :param filename: path to a newline-delimited json file
    :return: a generator of python objects, one for each line in the input file
    """
    with open(filename, 'rt') as f:
        for line in f.readlines():
            yield json.loads(line)


def convert_pandas_parallel(object_stream, converter_fn) -> Optional[pd.DataFrame]:
    """
    Takes a stream of objects and a converter_fn that converts each object to a pd.DataFrame.
    Maps objects from the input stream in parallel (up to num_cpus concurrently) and returns
    the results concatenated into one DataFrame
    """
    with mp.Pool(mp.cpu_count()) as pool:
        frames = pool.map(converter_fn, object_stream)
        if frames is None or len(frames) == 0:
            return None
    return pd.concat(frames, ignore_index=True)


def miner_state_to_df(j: dict) -> pd.DataFrame:
    chain_height = j['Height']
    tipset_key = j['TipsetKey']

    miners = j['MinerStates']
    for m in miners.values():
        df = pd.json_normalize(m)
        df['Height'] = chain_height
        df['TipsetKey'] = tipset_key
        df.rename(columns=MINER_STATE_COL_RENAMES, inplace=True)
    return df


class ChainDataFrames(object):
    """
    ChainDataFrames collects test output from a single test instance into pandas DataFrames.
    """

    # miner_states only includes state snapshots for tipsets that were included in the final chain
    miner_states: pd.DataFrame

    # transient_miner_states has state snapshots that were not included in the final chain (tipset reverted)
    transient_miner_states: pd.DataFrame

    # all_miner_states has all state snapshots, whether included in the final chain or not
    all_miner_states: pd.DataFrame

    # tipsets has all tipsets, whether included in the final chain or not
    tipsets: pd.DataFrame

    # included_tipsets has tipsets that were in the final chain (were never reverted)
    included_tipsets: pd.DataFrame

    # head_changes has a sequence of apply/revert operations that result in the final chain
    head_changes: pd.DataFrame

    def __init__(self, all_miner_states: pd.DataFrame, tipsets: pd.DataFrame, head_changes: pd.DataFrame, instance_id: Optional[str]):
        # annotate each tipset with whether it was included in the "final" chain
        # we determine this by summing up the "apply"/"revert" commands that have the same TipsetKey
        # using +1 for apply and -1 for revert. Any tipset with a positive score is considered included

        self.instance_id = instance_id
        if instance_id is not None:
            all_miner_states['test_instance'] = instance_id
            tipsets['test_instance'] = instance_id
            head_changes['test_instance'] = instance_id

        head_changes['apply_score'] = head_changes['Type'].apply(lambda t: -1 if t == 'revert' else 1)
        df = head_changes[['TipsetKey', 'apply_score']].groupby('TipsetKey').sum()

        joined = tipsets.join(df, on='TipsetKey')
        joined['included'] = joined['apply_score'] > 0
        tipsets['included'] = joined['included']

        joined = all_miner_states.join(df, on='TipsetKey')
        joined['included'] = joined['apply_score'] > 0
        all_miner_states['included'] = joined['included']

        if 'ChainHeight' in head_changes.columns and 'Height' not in head_changes.columns:
            head_changes['Height'] = head_changes['ChainHeight']

        self.miner_states = all_miner_states.where(all_miner_states['included']).dropna()
        self.transient_miner_states = all_miner_states.where(all_miner_states['included'] != True).dropna()
        self.all_miner_states = all_miner_states
        self.tipsets = tipsets
        self.included_tipsets = tipsets.where(tipsets['included']).dropna()
        self.head_changes = head_changes

    def to_pickle(self, dir_path: str):
        self.all_miner_states.to_pickle(os.path.join(dir_path, 'all_miner_states.gz'))
        self.tipsets.to_pickle(os.path.join(dir_path, 'tipsets.gz'))
        self.head_changes.to_pickle(os.path.join(dir_path, 'head_changes.gz'))

    @classmethod
    def read_pickle(cls, dir_path: str, instance_id: Optional[str]) -> 'ChainDataFrames':
        d = dict()
        for table in ['all_miner_states', 'tipsets', 'head_changes']:
            p = os.path.join(dir_path, '{}.gz'.format(table))
            if not os.path.exists(p):
                raise ValueError('no file found at ' + p)
            d[table] = pd.read_pickle(p)
        d['instance_id'] = instance_id
        return ChainDataFrames(**d)


    @classmethod
    def from_ndjson_files(cls, dir_path: str, instance_id: Optional[str]) -> 'ChainDataFrames':
        all_miner_states = cls.load_miner_state_ndjson(os.path.join(dir_path, 'chain-state.ndjson'))
        tipsets = cls.load_tipsets_ndjson(os.path.join(dir_path, 'chain-tipsets.ndjson'))
        head_changes = cls.load_head_changes_ndjson(os.path.join(dir_path, 'chain-head-changes.ndjson'))
        return ChainDataFrames(all_miner_states=all_miner_states, tipsets=tipsets, head_changes=head_changes,
                               instance_id=instance_id)

    @classmethod
    def load_miner_state_ndjson(cls, statefile: str) -> pd.DataFrame:
        miner_states = convert_pandas_parallel(read_ndjson(statefile), miner_state_to_df)
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
        if 'Sectors.Committed' in miner_states.columns:
            miner_states['CommittedSectors'] = miner_states['Sectors.Committed'].apply(count)
        if 'Sectors.Proving' in miner_states.columns:
            miner_states['ProvingSectors'] = miner_states['Sectors.Proving'].apply(count)
        return miner_states

    @classmethod
    def load_tipsets_ndjson(cls, tipset_file: str) -> pd.DataFrame:
        return convert_pandas_parallel(read_ndjson(tipset_file), pd.json_normalize)

    @classmethod
    def load_head_changes_ndjson(cls, head_change_file: str) -> pd.DataFrame:
        return convert_pandas_parallel(read_ndjson(head_change_file), pd.json_normalize)


def tick_formatter(col: str) -> Optional[str]:
    if col in ATTO_FIL_COLS:
        return '%8.4f FIL'
    return None


class ChainState(object):
    """
    ChainState represents a view of the blockchain from the perspective of one of the test instances.
    The actual data is represented by a ChainDataFrames instance, which contains pandas DataFrames
    derived from each instance's test output.
    """
    def __init__(self, pandas_data: ChainDataFrames):
        self.pandas = pandas_data

    @classmethod
    def from_ndjson_files(cls, dir_path: str, instance_id: Optional[str]):
        return ChainState(ChainDataFrames.from_ndjson_files(dir_path, instance_id=instance_id))

    @classmethod
    def from_pickle(cls, dir_path: str, instance_id: Optional[str]) -> 'ChainState':
        return ChainState(ChainDataFrames.read_pickle(dir_path, instance_id=instance_id))

    def to_pickle(self, dir_path):
        self.pandas.to_pickle(dir_path)

    def line_chart_selector_panel(self, variables: List[str] = None):
        """
        :param variables: a list of columns from the miner_states DataFrame. Defaults to all numeric columns.
        :return: a line chart with a dropdown selector that lets you chose one of the given variables to plot.
        The variable will be plotted on the Y axis with one series per miner, with chain height on the X axis.
        """
        if variables is None:
            variables = NUMERIC_COLS + DERIVED_COLS
        selector = pn.widgets.Select(name='Variable', options=variables)
        cols = ['Miner'] + variables
        plot = self.pandas.miner_states[cols].hvplot(by='Miner', y=selector)
        return pn.Column(pn.WidgetBox(selector), plot)

    def line_chart_stack(self, variables: List[str] = None):
        """
        :param variables: a list of columns from the miner_states DataFrame. Defaults to all numeric columns.
        :return: a vertical column of hvplot line charts, one for each variable in the list. The charts will plot
        one series for each miner, with the given variable on the Y axis and chain height on the X axis.
        """
        if variables is None:
            variables = NUMERIC_COLS + DERIVED_COLS

        plots = []
        for c in variables:
            title = c.split('.')[-1]
            p = self.pandas.miner_states[['Miner', c]].hvplot(by='Miner', y=c, title=title, yformatter=tick_formatter(c))
            plots.append(p)
        return pn.Column(*plots)

    def stacked_area(self, variable: str = 'Info.MinerPowerRaw', title: str = 'Miner Power Distribution (Raw)'):
        """
        :param variable: a column name from the miner_states data frame
        :param title: the chart title
        :return: a stacked area chart showing the proportion of `variable` across all miners on the Y axis
        in the range of (0.0, 1.0). The X axis is the height of the canonical blockchain.
        """
        df = self.pandas.miner_states[['Miner', variable]]
        df = df.pivot_table(values=[variable], index=df.index, columns='Miner', aggfunc='sum')
        df = df.div(df.sum(1), axis=0)
        df.columns = df.columns.get_level_values(1)
        return df.hvplot.area(title=title)

    def effective_height_table(self):
        """
        The "effective" height is the height after any revert operations have been applied, while the
        "logical" height is the expected height at a given epoch time. For example, if at epoch 3 we apply
        a tipset with height 3 and then revert it, the effective height would be 2 while the logical
        height remains 3. If we then apply a new tipset with height 3, the effective height becomes 3 again.

        Plotting the effective height lets us see the fluctuations as revert / apply operations happen -
        reorgs that span multiple epochs will cause a temporary divergence between expected and logical height
        as the tipsets from the abandoned fork are reverted and the effective height drops down to the lowest
        common ancestor before the fork, then climbs back up to the logical height as the tipsets from
        the heavier chain are applied.
        """
        hc = self.pandas.head_changes.copy()
        hc['effective_height'] = hc['Height'] + hc['apply_score'] - 1
        hc['ObservedAt'] = hc['ObservedAt'].apply(pd.to_datetime)
        hc.set_index('ObservedAt', inplace=True)
        return hc.drop(columns=['apply_score'])

    def head_change_step_chart(self):
        """
        :return: an hvplot step chart of the effective height of the chain over time,
        overlaid with the "logical" height (see effective_height_table for the difference).
        """
        hc = self.effective_height_table()

        cols = ['effective_height', 'TipsetKey']
        hover_cols = ['TipsetKey', 'ObservedAt']
        if 'test_instance' in hc.columns:
            cols.append('test_instance')
            hover_cols.append('test_instance')

        effective = hc[cols].hvplot.step(hover_cols=hover_cols)

        steps = hc['Height'].hvplot.step(legend=False, hover=False)
        return effective * steps


class TestResults(object):
    """
    TestResults represents the output from a single test run. It includes data from all participants.
    """

    def __init__(self, output_dir_or_archive: str):
        if os.path.isdir(output_dir_or_archive):
            self.output_dir = output_dir_or_archive
        else:
            self.temp_dir = archive_temp_dir()
            extract_archive(output_dir_or_archive, self.temp_dir.name)
            inner = os.listdir(self.temp_dir.name)[0]
            self.output_dir = os.path.join(self.temp_dir.name, inner)

        groups = []
        instances = []
        with os.scandir(self.output_dir) as it:
            for entry in it:
                if entry.is_dir():
                    groups.append(entry.name)

        for g in groups:
            with os.scandir(os.path.join(self.output_dir, g)) as it:
                for entry in it:
                    if entry.is_dir():
                        instances.append('{}/{}'.format(g, entry.name))

        self.groups = groups
        self.instances = instances
        self.states = dict()
        self._load()

    def _load(self):
        for i in self.instances:
            if i in self.states:
                continue
            p = os.path.join(self.output_dir, i)
            if not os.path.exists(os.path.join(p, 'chain-state.ndjson')):
                print('no state found for {}, ignoring'.format(i))
                continue
            print('loading state for {}'.format(i))
            with benchmark(i):
                cs = ChainState.from_ndjson_files(p, instance_id=i)
                self.states[i] = cs

    def cleanup(self):
        if self.temp_dir is not None:
            self.temp_dir.cleanup()

    def effective_heights(self) -> pd.DataFrame:
        df: pd.DataFrame = None
        for i, cs in self.states.items():
            if df is None:
                df = cs.effective_height_table()
            else:
                df = df.append(cs.effective_height_table())
        return df

    def head_change_step_chart(self):
        cols = ['effective_height', 'test_instance']
        eh = self.effective_heights()
        effective = eh[cols].hvplot.step(by='test_instance')
        logical = eh[['Height']].hvplot.step()
        return logical * effective


