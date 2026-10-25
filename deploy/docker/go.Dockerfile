# 所有 Go 服务共用此文件，CMD_NAME 对应 cmd/ 下的程序名；固定编译器与 Alpine 版本。
FROM golang:1.27.1-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG CMD_NAME=inventory-service
ARG APP_VERSION=dev
ARG GO_TAGS=
# 纯 Go 二进制无需运行镜像提供 libc；裁掉调试符号减小本机镜像体积。
RUN CGO_ENABLED=0 go build -tags="${GO_TAGS}" -trimpath -ldflags="-s -w -X github.com/Li-Nepenthe/opsdoctor/internal/platform/telemetry.Version=${APP_VERSION}" -o /out/server ./cmd/${CMD_NAME}

FROM alpine:3.24.2
# 非 root 用户运行服务；BusyBox 的 wget 用于本机 /health 探测，不额外安装健康检查程序。
RUN addgroup -S app && adduser -S -G app app
COPY --from=build /out/server /app/server
USER app
ENTRYPOINT ["/app/server"]
