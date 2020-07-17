FROM golang:1.14.4-buster as tg-build

ARG TESTGROUND_REF="oni"
WORKDIR /usr/src
RUN git clone https://github.com/testground/testground.git
RUN cd testground && git checkout $TESTGROUND_REF && go build .

FROM python:3.8-buster

WORKDIR /usr/src/app

COPY --from=tg-build /usr/src/testground/testground /usr/bin/testground

RUN mkdir /composer && chmod 777 /composer
RUN mkdir /testground && chmod 777 /testground

ENV HOME /composer
ENV TESTGROUND_HOME /testground
ENV LISTEN_PORT 5006
ENV TESTGROUND_DAEMON_HOST host.docker.internal

VOLUME /testground/plans


COPY requirements.txt ./
RUN pip install -r requirements.txt

# for serving the composer ui as a standalone webapp:
# CMD panel serve --address 0.0.0.0 --port $LISTEN_PORT composer.ipynb

# full jupyter server. uses a fixed token so we don't have to extract it at runtime.
CMD jupyter notebook --ip=0.0.0.0 --NotebookApp.token='68be29c4c1a8451784252ffc6c936593'