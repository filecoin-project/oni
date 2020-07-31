# Analysing Oni Tests with Jupyter

This directory has some tools for examining the output of a test using Jupyter.

The `app` directory contains python code that's meant to be imported into a Jupyter notebook.

The most useful code is in `app/chainstate.py` - it will load JSON chain state dumps from lotus-soup tests
that use the `UpdateChainState` routine to dump chain state.

The code is quite messy, so will likely take work to be useful.

If you want to run without docker, make a python 3.8+ virtualenv and run `pip install -r requirements.txt`.
Then you can run `jupyter notebook` in this directory.

## Docker madness

To avoid python dependency madness, there's a `Dockerfile` in here that will setup a python environment
and install everything from `requirements.txt` and mount the `app` code in.

To use it, run `./jupyter.sh` in this repo. The first time you run it may take a while as the images are built.

Once the jupyter server is running in docker, the script should open the Jupyter UI in your default browser.
Since the current directory is mounted into the container, any edits you make to the python code locally will
show up, and you can access test outputs, etc from the Jupyter environment.

## Misc stuff you can probably ignore

There's some not-quite-baked code in here for creating testground compositions using a data-driven UI. It was
close to being cool, but didn't quite get there. Maybe someday :)