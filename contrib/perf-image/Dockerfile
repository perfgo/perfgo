# This stages builds the gimli-addr2line tool as static binary
FROM clux/muslrust:1.92.0-stable AS addr2line

RUN cargo install addr2line --features bin


# This is the main image
FROM debian:trixie

RUN \
  apt-get update && \
  apt-get install -y linux-perf openssh-server && \
  rm -v /etc/ssh/ssh_host_* && \
  apt-get clean

COPY --from=addr2line /opt/cargo/bin/addr2line /usr/local/bin/addr2line
