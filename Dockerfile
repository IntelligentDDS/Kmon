FROM harbor.dds-sysu.tech/ebpf_env/ebpf_env:latest as builder
WORKDIR /build
COPY deployment ./deployment
RUN bash ./deployment/script/install_go.sh
COPY . .
ENV PATH="/usr/local/go/bin:${PATH}"
RUN make


FROM harbor.dds-sysu.tech/ebpf_env/ebpf_env:latest
WORKDIR ./ebpf
COPY --from=builder /build/release .
RUN ls
ENTRYPOINT ["./kmon"]