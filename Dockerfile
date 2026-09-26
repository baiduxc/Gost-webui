# ---------- 面板构建 ----------
FROM golang:1.26-alpine AS panel
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gost-webui .

# ---------- gost 构建（源码 master：含 vmess / API 配额，官方发行版无 vmess） ----------
FROM golang:1.26-alpine AS gostbin
RUN apk add --no-cache git
WORKDIR /gost
ARG GOST_REF=master
RUN git clone --depth 1 --branch ${GOST_REF} https://github.com/go-gost/gost.git . && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gost ./cmd/gost

# ---------- sing-box（VLESS+REALITY 引擎，官方预编译，多架构） ----------
FROM alpine:3.20 AS singbox
ARG SINGBOX_VERSION=1.14.2
ARG TARGETARCH
RUN apk add --no-cache curl && \
    curl -fsSL "https://github.com/SagerNet/sing-box/releases/download/v${SINGBOX_VERSION}/sing-box-${SINGBOX_VERSION}-linux-${TARGETARCH}.tar.gz" -o /sb.tgz && \
    tar -xzf /sb.tgz -C / && \
    cp /sing-box-${SINGBOX_VERSION}-linux-${TARGETARCH}/sing-box /sing-box && chmod +x /sing-box

# ---------- 运行时 ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -S gost && adduser -S gost -G gost
ENV TZ=Asia/Shanghai
COPY --from=panel /out/gost-webui /usr/local/bin/gost-webui
COPY --from=gostbin /out/gost /usr/local/bin/gost
COPY --from=singbox /sing-box /usr/local/bin/sing-box
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh && \
    mkdir -p /var/lib/gost-webui /var/log/gost-webui /etc/gost-webui && \
    chown -R gost:gost /var/lib/gost-webui /var/log/gost-webui /etc/gost-webui
USER gost
VOLUME ["/var/lib/gost-webui", "/var/log/gost-webui", "/etc/gost-webui"]
EXPOSE 8787
ENTRYPOINT ["/entrypoint.sh"]
