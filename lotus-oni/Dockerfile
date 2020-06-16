ARG GO_VERSION=1.14.2



FROM golang:${GO_VERSION}-buster AS lotus-oni-base
RUN apt update && apt install -y mesa-opencl-icd ocl-icd-opencl-dev gcc git bzr jq pkg-config mesa-opencl-icd ocl-icd-opencl-dev
### Download IPFS binaries
RUN cd /go && curl https://ipfs.io/ipns/dist.ipfs.io/go-ipfs/v0.5.1/go-ipfs_v0.5.1_linux-amd64.tar.gz -o ipfs.tar.gz && tar -xzvvf ipfs.tar.gz && mv /go/go-ipfs/ipfs /go/bin/



FROM lotus-oni-base AS lotus-oni
ARG LOTUS_VERSION=master
RUN git clone https://github.com/filecoin-project/lotus.git && cd lotus && git checkout ${LOTUS_VERSION} && make clean && make all && go build -o lotus-seed ./cmd/lotus-seed
RUN mv /go/lotus /lotus/



FROM lotus-oni-base AS builder
ARG TESTPLAN_EXEC_PKG="."
ARG GO_PROXY=http://localhost:8081
ARG BUILD_TAGS
ENV TESTPLAN_EXEC_PKG ${TESTPLAN_EXEC_PKG}
ENV PLAN_DIR /plan/
COPY /plan/go.mod ${PLAN_DIR}
### Copy Lotus source
COPY --from=lotus-oni /lotus /lotus
RUN echo "Using go proxy: ${GO_PROXY}" \
    && cd ${PLAN_DIR} \
    && go env -w GOPROXY="${GO_PROXY}" \
    && go mod download
COPY . /
RUN cd ${PLAN_DIR} \
    && go env -w GOPROXY="${GO_PROXY}" \
    && GOOS=linux GOARCH=amd64 go build -o testplan ${BUILD_TAGS} ${TESTPLAN_EXEC_PKG}
RUN cd ${PLAN_DIR} \
  && go list -m all > /testground_dep_list



FROM lotus-oni-base
COPY --from=builder /testground_dep_list /
COPY --from=builder /plan/testplan /
### Copy Lotus source and binaries
COPY --from=lotus-oni /lotus /lotus
COPY --from=lotus-oni-base /go/bin/ipfs /go/bin/ipfs
EXPOSE 6060
ENTRYPOINT [ "/testplan"]
